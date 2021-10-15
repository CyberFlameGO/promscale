// This file and its contents are licensed under the Apache License 2.0.
// Please see the included NOTICE for copyright information and
// LICENSE for a copy of the license.

package trace

import (
	"context"
	"fmt"

	"github.com/jackc/pgtype"
	pgx "github.com/jackc/pgx/v4"
	"github.com/timescale/promscale/pkg/clockcache"
	"github.com/timescale/promscale/pkg/pgmodel/common/schema"
	"github.com/timescale/promscale/pkg/pgxconn"
)

const insertInstrumentationLibSQL = `SELECT %s.put_instrumentation_lib($1, $2, $3)`

type instrumentationLibrary struct {
	name        string
	version     string
	schemaURLID pgtype.Int8
}

func (il instrumentationLibrary) SizeInCache() uint64 {
	return uint64(len(il.name) + len(il.version) + 18) // 9 bytes per pgtype.Int8
}

func (il instrumentationLibrary) Before(item queable) bool {
	otherIl, ok := item.(instrumentationLibrary)
	if !ok {
		return false
	}
	if il.name != otherIl.name {
		return il.name < otherIl.name
	}
	if il.version != otherIl.version {
		return il.version < otherIl.version
	}
	if il.schemaURLID.Status != otherIl.schemaURLID.Status {
		return il.schemaURLID.Status < otherIl.schemaURLID.Status
	}
	return il.schemaURLID.Int < otherIl.schemaURLID.Int
}

func (il instrumentationLibrary) Queries() ([]string, [][]interface{}) {
	return []string{fmt.Sprintf(insertInstrumentationLibSQL, schema.TracePublic)},
		[][]interface{}{{il.name, il.version, il.schemaURLID}}
}

func (il instrumentationLibrary) Result(r pgx.BatchResults) (interface{}, error) {
	var id pgtype.Int8
	err := r.QueryRow().Scan(&id)
	return id, err
}

// instrumentationLibraryBatch queues up items to send to the db but it sorts before sending
//this avoids deadlocks in the db
type instrumentationLibraryBatch struct {
	b batcher
}

func newInstrumentationLibraryBatch(cache *clockcache.Cache) instrumentationLibraryBatch {
	return instrumentationLibraryBatch{
		b: newBatcher(cache),
	}
}

func (ilb instrumentationLibraryBatch) Queue(name, version string, schemaURLID pgtype.Int8) {
	if name == "" {
		return
	}
	ilb.b.Queue(instrumentationLibrary{name, version, schemaURLID})
}

func (ilb instrumentationLibraryBatch) SendBatch(ctx context.Context, conn pgxconn.PgxConn) (err error) {
	return ilb.b.SendBatch(ctx, conn)
}
func (ilb instrumentationLibraryBatch) GetID(name, version string, schemaURLID pgtype.Int8) (pgtype.Int8, error) {

	if name == "" {
		return pgtype.Int8{Status: pgtype.Null}, nil
	}
	il := instrumentationLibrary{name, version, schemaURLID}
	id, err := ilb.b.GetInt8(il)
	if err != nil {
		return pgtype.Int8{Status: pgtype.Null}, fmt.Errorf("error getting ID for instrumentation library %v: %w", il, err)
	}

	return id, nil
}
