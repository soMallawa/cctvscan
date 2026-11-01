package store

import (
	"context"
	"encoding/json"
	"github.com/jackc/pgx/v5"
)

// ExportResults streams a consistent job/evidence snapshot without collecting
// the complete report in memory. Callback errors abort the read transaction.
func (s *Store) ExportResults(ctx context.Context, id string, begin func(JobDTO) error, emit func(json.RawMessage) error) error {
	tx, e := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	j, e := scanJob(tx.QueryRow(ctx, `SELECT `+jobColumns+` FROM jobs WHERE id=$1`, id))
	if e != nil {
		return e
	}
	rows, e := tx.Query(ctx, `SELECT jsonb_build_object('id',id::text,'job_id',job_id,'target',target,'brand',brand,'state',state,'result',data) FROM results WHERE job_id=$1 ORDER BY id`, id)
	if e != nil {
		return e
	}
	defer rows.Close()
	if e = begin(j); e != nil {
		return e
	}
	for rows.Next() {
		var data json.RawMessage
		if e = rows.Scan(&data); e != nil {
			return e
		}
		if e = emit(data); e != nil {
			return e
		}
	}
	if e = rows.Err(); e != nil {
		return e
	}
	rows.Close()
	return tx.Commit(ctx)
}
