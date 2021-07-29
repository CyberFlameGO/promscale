// This file and its contents are licensed under the Apache License 2.0.
// Please see the included NOTICE for copyright information and
// LICENSE for a copy of the license.

package ingestor

import (
	"context"
	"fmt"

	"github.com/timescale/promscale/pkg/log"
	"github.com/timescale/promscale/pkg/pgmodel/cache"
	"github.com/timescale/promscale/pkg/pgmodel/common/errors"
	"github.com/timescale/promscale/pkg/pgmodel/common/schema"
	"github.com/timescale/promscale/pkg/pgmodel/model"
	"github.com/timescale/promscale/pkg/pgxconn"
)

const (
	getCreateMetricsTableWithNewSQL = "SELECT table_name, possibly_new FROM " + schema.Catalog + ".get_or_create_metric_table_name($1)"
)

type metricBatcher struct {
	conn            pgxconn.PgxConn
	input           chan *insertDataRequest
	pending         *pendingBuffer
	metricTableName string
	toCopiers       chan<- copyRequest
}

func metricTableName(conn pgxconn.PgxConn, metric string) (string, bool, error) {
	res, err := conn.Query(
		context.Background(),
		getCreateMetricsTableWithNewSQL,
		metric,
	)

	if err != nil {
		return "", true, fmt.Errorf("failed to get the table name for metric %s: %w", metric, err)
	}

	var tableName string
	var possiblyNew bool
	defer res.Close()
	if !res.Next() {
		if err := res.Err(); err != nil {
			return "", true, fmt.Errorf("failed to get the table name for metric %s: %w", metric, err)
		}
		return "", true, errors.ErrMissingTableName
	}

	if err := res.Scan(&tableName, &possiblyNew); err != nil {
		return "", true, fmt.Errorf("failed to get the table name for metric %s: %w", metric, err)
	}

	if err := res.Err(); err != nil {
		return "", true, fmt.Errorf("failed to get the table name for metric %s: %w", metric, err)
	}

	if tableName == "" {
		return "", true, fmt.Errorf("failed to get the table name for metric %s: empty table name returned", metric)
	}

	return tableName, possiblyNew, nil
}

// Create the metric table for the metric we handle, if it does not already
// exist. This only does the most critical part of metric table creation, the
// rest is handled by completeMetricTableCreation().
func initializeMetricBatcher(conn pgxconn.PgxConn, metricName string, completeMetricCreationSignal chan struct{}, metricTableNames cache.MetricCache) (tableName string, err error) {
	mInfo, err := metricTableNames.Get(schema.Data, metricName)
	if err == nil && mInfo.TableName != "" {
		return mInfo.TableName, nil
	}

	tableName, possiblyNew, err := metricTableName(conn, metricName)
	if err != nil || tableName == "" {
		return "", err
	}

	//ignore error since this is just an optimization
	_ = metricTableNames.Set(
		schema.Data,
		metricName,
		model.MetricInfo{
			TableSchema: schema.Data, TableName: tableName,
			SeriesTable: "",
		},
	)

	if possiblyNew {
		//pass a signal if there is space
		select {
		case completeMetricCreationSignal <- struct{}{}:
		default:
		}
	}
	return tableName, err
}

func runMetricBatcher(conn pgxconn.PgxConn,
	input chan *insertDataRequest,
	metricName string,
	completeMetricCreationSignal chan struct{},
	metricTableNames cache.MetricCache,
	toCopiers chan<- copyRequest,
	labelArrayOID uint32) {

	var tableName string
	var firstReq *insertDataRequest
	firstReqSet := false
	for firstReq = range input {
		var err error
		tableName, err = initializeMetricBatcher(conn, metricName, completeMetricCreationSignal, metricTableNames)
		if err != nil {
			err := fmt.Errorf("initializing the insert routine for metric %v has failed with %w", metricName, err)
			log.Error("msg", err)
			firstReq.reportResult(err)
		} else {
			firstReqSet = true
			break
		}
	}

	//input channel was closed before getting a successful request
	if !firstReqSet {
		return
	}

	handler := metricBatcher{
		conn:            conn,
		input:           input,
		pending:         NewPendingBuffer(),
		metricTableName: tableName,
		toCopiers:       toCopiers,
	}

	handler.handleReq(firstReq)

	// Grab new requests from our channel and handle them. We do this hot-load
	// style: we keep grabbing requests off the channel while we can do so
	// without blocking, and flush them to the next layer when we run out, or
	// reach a predetermined threshold. The theory is that wake/sleep and
	// flushing is relatively expensive, and can be easily amortized over
	// multiple requests, so it pays to batch as much as we are able. However,
	// writes to a given metric can be relatively rare, so if we don't have
	// additional requests immediately we're likely not going to for a while.
	for {
		if handler.pending.IsEmpty() {
			stillAlive := handler.blockingHandleReq()
			if !stillAlive {
				return
			}
			continue
		}

	hotReceive:
		for handler.nonblockingHandleReq() {
			if handler.pending.IsFull() {
				break hotReceive
			}
		}

		handler.flush()
	}
}

func (h *metricBatcher) blockingHandleReq() bool {
	req, ok := <-h.input
	if !ok {
		return false
	}

	h.handleReq(req)

	return true
}

func (h *metricBatcher) nonblockingHandleReq() bool {
	select {
	case req := <-h.input:
		h.handleReq(req)
		return true
	default:
		return false
	}
}

func (h *metricBatcher) handleReq(req *insertDataRequest) bool {
	h.pending.addReq(req)
	if h.pending.IsFull() {
		h.flushPending()
		return true
	}
	return false
}

func (h *metricBatcher) flush() {
	if h.pending.IsEmpty() {
		return
	}
	h.flushPending()
}

// Set all unset SeriesIds and flush to the next layer
func (h *metricBatcher) flushPending() {
	MetricBatcherFlushSeries.Observe(float64(h.pending.batch.CountSeries()))
	h.toCopiers <- copyRequest{h.pending, h.metricTableName}
	h.pending = NewPendingBuffer()
}
