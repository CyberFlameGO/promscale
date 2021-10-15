package trace

import (
	"context"
	"fmt"
	"sort"

	"github.com/jackc/pgtype"
	pgx "github.com/jackc/pgx/v4"
	"github.com/timescale/promscale/pkg/clockcache"
	"github.com/timescale/promscale/pkg/pgmodel/common/errors"
	"github.com/timescale/promscale/pkg/pgxconn"
)

type queable interface {
	Queries() ([]string, [][]interface{})
	Before(queable) bool
	Result(pgx.BatchResults) (interface{}, error)
	SizeInCache() uint64
}

type queables []queable

func (q queables) Len() int {
	return len(q)
}

func (q queables) Less(i, j int) bool {
	return q[i].Before(q[j])
}

func (q queables) Swap(i, j int) {
	q[i], q[j] = q[j], q[i]
}

func (b batcher) Queue(i queable) {
	b.batch[i] = nil
}

//batcher queues up items to send to the DB but it sorts before sending
//this avoids deadlocks in the DB. It also avoids sending the same items repeatedly.
type batcher struct {
	batch map[queable]interface{}
	cache *clockcache.Cache
}

func newBatcher(cache *clockcache.Cache) batcher {
	return batcher{
		batch: make(map[queable]interface{}),
		cache: cache,
	}
}

func (b batcher) SendBatch(ctx context.Context, conn pgxconn.PgxConn) (err error) {
	batch := make([]queable, len(b.batch))
	i := 0
	for q := range b.batch {
		id, ok := b.cache.Get(q)
		if !ok {
			batch[i] = q
			i++
			continue
		}
		b.batch[q] = id
	}

	if i == 0 { // All urls cached.
		return nil
	}

	batch = batch[:i]

	sort.Sort(queables(batch))

	dbBatch := conn.NewBatch()
	for _, item := range batch {
		sqls, args := item.Queries()
		for i, sql := range sqls {
			dbBatch.Queue(sql, args[i]...)
		}
	}

	br, err := conn.SendBatch(ctx, dbBatch)
	if err != nil {
		return err
	}

	defer func() {
		// Only return Close error if there was no previous error.
		if tempErr := br.Close(); err == nil {
			err = tempErr
		}
	}()

	for _, item := range batch {
		id, err := item.Result(br)
		if err != nil {
			return err
		}
		b.batch[item] = id
		b.cache.Insert(item, id, item.SizeInCache())
	}

	return nil
}

func (b batcher) GetInt8(i queable) (pgtype.Int8, error) {
	entry, ok := b.batch[i]
	if !ok {
		return pgtype.Int8{Status: pgtype.Null}, fmt.Errorf("error getting ID from batch")
	}

	id, ok := entry.(pgtype.Int8)
	if !ok {
		return pgtype.Int8{Status: pgtype.Null}, errors.ErrInvalidCacheEntryType
	}
	if id.Status != pgtype.Present {
		return pgtype.Int8{Status: pgtype.Null}, fmt.Errorf("ID is null")
	}
	if id.Int == 0 {
		return pgtype.Int8{Status: pgtype.Null}, fmt.Errorf("ID is 0")
	}
	return id, nil
}

func (b batcher) Get(i queable) (interface{}, error) {
	entry, ok := b.batch[i]
	if !ok {
		return nil, fmt.Errorf("error getting item from batch")
	}

	return entry, nil
}
