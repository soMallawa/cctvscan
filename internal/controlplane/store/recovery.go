package store

import (
	"context"
	"encoding/json"
)

// RecoverOrphans fences stale attempts; it never requeues potentially
// side-effecting work or turns an unknown stop into a successful cancellation.
func (s *Store) RecoverOrphans(ctx context.Context) error {
	rows, e := s.pool.Query(ctx, `SELECT j.id::text FROM jobs j JOIN attempts a ON a.job_id=j.id AND a.number=j.attempt JOIN runners r ON r.id=a.runner_id WHERE j.state IN ('running','canceling') AND a.state='running' AND r.heartbeat_at<now()-interval '30 seconds' ORDER BY j.created_at LIMIT 50`)
	if e != nil {
		return e
	}
	var ids []string
	for rows.Next() {
		var id string
		if e = rows.Scan(&id); e != nil {
			rows.Close()
			return e
		}
		ids = append(ids, id)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	for _, id := range ids {
		if e = func() error {
			tx, e := s.pool.Begin(ctx)
			if e != nil {
				return e
			}
			defer tx.Rollback(ctx)
			tag, e := tx.Exec(ctx, `UPDATE jobs j SET state='orphaned',phase='orphaned',finished_at=now(),error='Runner heartbeat expired; execution coverage is uncertain.' WHERE j.id=$1 AND j.state IN ('running','canceling') AND EXISTS(SELECT 1 FROM attempts a JOIN runners r ON r.id=a.runner_id WHERE a.job_id=j.id AND a.number=j.attempt AND a.state='running' AND r.heartbeat_at<now()-interval '30 seconds')`, id)
			if e != nil {
				return e
			}
			if tag.RowsAffected() == 0 {
				return nil
			}
			if _, e = tx.Exec(ctx, `UPDATE attempts SET state='orphaned',finished_at=now() WHERE job_id=$1 AND state='running'`, id); e != nil {
				return e
			}
			if _, e = s.AddEventTx(ctx, tx, id, "state", json.RawMessage(`{"state":"orphaned","message":"Runner heartbeat expired; no automatic retry was performed."}`)); e != nil {
				return e
			}
			return tx.Commit(ctx)
		}(); e != nil {
			return e
		}
	}
	// A canceled River delivery can leave a canceling, never-started application
	// job. No process can have started before attempt=1 was committed.
	rows, e = s.pool.Query(ctx, `SELECT j.id::text FROM jobs j JOIN river_job r ON r.id=j.river_job_id WHERE j.state='canceling' AND j.cancel_requested AND j.attempt=0 AND r.state IN ('cancelled','completed','discarded') ORDER BY j.created_at LIMIT 50`)
	if e != nil {
		return e
	}
	ids = ids[:0]
	for rows.Next() {
		var id string
		if e = rows.Scan(&id); e != nil {
			rows.Close()
			return e
		}
		ids = append(ids, id)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	for _, id := range ids {
		if e = func() error {
			tx, e := s.pool.Begin(ctx)
			if e != nil {
				return e
			}
			defer tx.Rollback(ctx)
			tag, e := tx.Exec(ctx, `UPDATE jobs SET state='canceled',phase='canceled',finished_at=now() WHERE id=$1 AND state='canceling' AND cancel_requested AND attempt=0`, id)
			if e != nil {
				return e
			}
			if tag.RowsAffected() == 0 {
				return nil
			}
			if _, e = s.AddEventTx(ctx, tx, id, "state", json.RawMessage(`{"state":"canceled","cancel_requested":true}`)); e != nil {
				return e
			}
			return tx.Commit(ctx)
		}(); e != nil {
			return e
		}
	}
	return nil
}
