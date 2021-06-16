package bench

import (
	"fmt"
	"time"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/record"

	"github.com/prometheus/prometheus/tsdb"
)

const (
	/* copied from queue_manager.go */
	ewmaWeight          = 0.2
	shardUpdateDuration = 10 * time.Second

	//from cli flag storage.remote.flush-deadline in main.go
	//How long to wait flushing sample on shutdown or config reload.
	defaultFlushDeadline = 1 * time.Minute

	WALSimulatorChannelSize = 100000000
)

func checkSeriesSet(ss storage.SeriesSet) error {
	if ws := ss.Warnings(); len(ws) > 0 {
		return ws[0]
	}
	if ss.Err() != nil {
		return ss.Err()
	}
	return nil
}

func getSeriesID(conf *BenchConfig, rawSeriesID uint64, seriesMultipliedIndex int) (seriesID uint64) {
	return (rawSeriesID * uint64(conf.SeriesMultiplier)) + uint64(seriesMultipliedIndex)
}

func RunFullSimulation(conf *BenchConfig, qmi *qmInfo, q storage.Querier, ws *walSimulator, runNumber int) (time.Time, int, error) {
	ss := q.Select(false, nil, labels.MustNewMatcher(labels.MatchRegexp, "", ".*"))

	sth, err := NewSeriesTimeHeap(conf, ss, qmi, seriesIndex)
	if err != nil {
		return time.Time{}, 0, err
	}

	start := time.Now()
	startTs := int64(model.TimeFromUnixNano(start.UnixNano()))
	firstTs := sth[0].ts
	dataTimeStartTs := firstTs
	if conf.UseWallClockForDataTime {
		dataTimeStartTs = startTs
	}

	count := 0
	err = sth.Visit(func(rawTs int64, val float64, seriesID uint64) error {
		timestampDelta := int64(float64(rawTs-firstTs) / conf.RateMultiplier)
		dataTimestamp := dataTimeStartTs + timestampDelta
		wallTimestamp := startTs + timestampDelta

		if conf.RateControl {
			wait := time.Until(model.Time(wallTimestamp).Time())
			if wait > 0 {
				time.Sleep(wait)
			}
		}
		samplesSent := 0
		for seriesMultiplierIndex := 0; seriesMultiplierIndex < conf.SeriesMultiplier; seriesMultiplierIndex++ {
			samples := []record.RefSample{
				{
					Ref: getSeriesID(conf, seriesID, seriesMultiplierIndex),
					T:   dataTimestamp,
					V:   val,
				},
			}
			ws.Append(samples)
			samplesSent++
		}
		count += samplesSent
		qmi.highestTs.Set(float64(dataTimestamp / 1000))
		qmi.samplesIn.Incr(int64(samplesSent))
		return nil
	})
	if err == nil {
		err = checkSeriesSet(ss)
	}
	return start, count, err
}

var seriesIndex = 0

func Run(conf *BenchConfig) (err error) {
	db, err := tsdb.OpenDBReadOnly(conf.TSDBPath, nil)
	if err != nil {
		return err
	}
	defer func() {
		err2 := db.Close()
		if err == nil {
			err = err2
		}
	}()

	blocks, err := db.Blocks()
	if err != nil {
		return err
	}
	fmt.Println("Number of Blocks", len(blocks))

	qmi, err := getQM(conf)
	if err != nil {
		return err
	}
	qmi.qm.Start()

	ws := NewWalSimulator(qmi)
	totalCount := 0
	start := time.Time{}
	for i := 0; i < conf.RepeatedRuns; i++ {
		var runDuration time.Duration
		runCount := 0
		for blockIdx := 0; blockIdx < len(blocks); blockIdx++ {
			err := func() error {
				block := blocks[blockIdx].(*tsdb.Block)
				fmt.Println("Starting processing block", block.Meta().MinTime, block.Meta().MaxTime)
				q, err := tsdb.NewBlockQuerier(block, conf.Mint, conf.Maxt)
				if err != nil {
					return err
				}
				defer q.Close()

				blockStart, blockCount, err := RunFullSimulation(conf, qmi, q, ws, i)
				if err != nil {
					return err
				}
				if start.IsZero() {
					start = blockStart
				}
				totalCount += blockCount
				runCount += blockCount
				ewmaRateSent := qmi.samplesIn.Rate()
				blockDuration := time.Since(blockStart)
				runDuration += blockDuration
				fmt.Println("single block took", blockDuration, "count", blockCount, "ewma metric/s sent", ewmaRateSent, "metrics/s db", float64(blockCount)/blockDuration.Seconds())

				qmi.qm.SeriesReset(seriesIndex)
				seriesIndex++
				return nil
			}()
			if err != nil {
				return err
			}
			//runtime.GC()
		}
		fmt.Println("single run took", runDuration, "count", runCount, "metrics/s db", float64(runCount)/runDuration.Seconds())

	}

	qmi.markStoppedSend()

	ewmaRateSent := qmi.samplesIn.Rate()
	ws.Stop()
	qmi.qm.Stop()
	took := time.Since(start)
	fmt.Println("took", took, "count", totalCount, "ewma metric/s sent", ewmaRateSent, "metrics/s db", float64(totalCount)/took.Seconds())
	return nil
}