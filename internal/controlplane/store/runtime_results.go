package store

import (
	"context"
	"encoding/json"
)

type ResultInput struct {
	Target, Brand, State string
	Data                 json.RawMessage
	Camera               bool
	Streams              int
	Inconclusive         bool
}

// SaveResults accepts bounded, pre-sanitized scanner records, fences the write,
// and advances measured counters only for newly persisted host identities.
func (s *Store) SaveResults(ctx context.Context, a Attempt, results []ResultInput) error {
	if len(results) > 50 {
		return ErrInvalidInput
	}
	tx, e := s.pool.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	var current bool
	if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM attempts a WHERE a.job_id=j.id AND a.id=$2 AND a.fencing_token=$3 AND a.state='running' AND a.number=j.attempt) AND j.state IN ('running','canceling') FROM jobs j WHERE j.id=$1 FOR UPDATE`, a.JobID, a.ID, a.Token).Scan(&current); e != nil {
		return e
	}
	if !current {
		return ErrFenced
	}
	count, cameras, streams, inconclusive := 0, 0, 0, 0
	for _, r := range results {
		if r.Target == "" || len(r.Target) > 64 || len(r.Brand) > 200 || !json.Valid(r.Data) || len(r.Data) > 1048576 || r.Streams < 0 {
			return ErrInvalidInput
		}
		if r.State != "complete" && r.State != "inconclusive" && r.State != "canceled" {
			return ErrInvalidInput
		}
		tag, e := tx.Exec(ctx, `INSERT INTO results(job_id,attempt_id,target,brand,state,data) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(job_id,attempt_id,target) DO NOTHING`, a.JobID, a.ID, r.Target, r.Brand, r.State, r.Data)
		if e != nil {
			return e
		}
		if tag.RowsAffected() == 1 {
			count++
			if r.Camera {
				cameras++
			}
			streams += r.Streams
			if r.Inconclusive {
				inconclusive++
			}
		}
	}
	if count > 0 {
		if _, e = tx.Exec(ctx, `UPDATE jobs SET result_count=result_count+$1,camera_count=camera_count+$2,stream_count=stream_count+$3,inconclusive_count=inconclusive_count+$4 WHERE id=$5`, count, cameras, streams, inconclusive, a.JobID); e != nil {
			return e
		}
		data, _ := json.Marshal(map[string]any{"hosts_imported": count})
		if _, e = s.AddEventTx(ctx, tx, a.JobID, "metrics", data); e != nil {
			return e
		}
	}
	return tx.Commit(ctx)
}
func (s *Store) Phase(ctx context.Context, a Attempt, phase string) error {
	if phase != "scanning" && phase != "importing" && phase != "port_discovery" && phase != "host_processing" && phase != "stopping" {
		return ErrInvalidInput
	}
	tx, e := s.pool.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	tag, e := tx.Exec(ctx, `UPDATE jobs j SET phase=$1 WHERE j.id=$2 AND j.state IN ('running','canceling') AND EXISTS(SELECT 1 FROM attempts a WHERE a.job_id=j.id AND a.id=$3 AND a.fencing_token=$4 AND a.number=j.attempt AND a.state='running')`, phase, a.JobID, a.ID, a.Token)
	if e != nil {
		return e
	}
	if tag.RowsAffected() != 1 {
		return ErrFenced
	}
	data, _ := json.Marshal(map[string]string{"phase": phase})
	if _, e = s.AddEventTx(ctx, tx, a.JobID, "phase", data); e != nil {
		return e
	}
	return tx.Commit(ctx)
}
