package store

import (
	"context"
	"encoding/json"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"io"
	"log/slog"
	"time"
)

func (s *Store) WorkerClient(worker river.Worker[ScanJobArgs]) (*river.Client[pgx.Tx], error) {
	workers := river.NewWorkers()
	river.AddWorker(workers, worker)
	return river.NewClient(riverpgxv5.New(s.pool), &river.Config{Workers: workers, Queues: map[string]river.QueueConfig{river.QueueDefault: {MaxWorkers: 1}}, JobTimeout: 25 * time.Hour, RescueStuckJobsAfter: 26 * time.Hour, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
}
func (s *Store) RunnerHeartbeat(ctx context.Context, id string, stats json.RawMessage) error {
	if !json.Valid(stats) || len(stats) > 16384 {
		return ErrInvalidInput
	}
	tag, e := s.pool.Exec(ctx, `UPDATE runners SET heartbeat_at=now(),stats=$2 WHERE id=$1`, id, stats)
	if e == nil && tag.RowsAffected() != 1 {
		return ErrNotFound
	}
	return e
}
