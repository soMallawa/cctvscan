package store

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/postfix/cctvscan/internal/controlplane/config"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivertype"
	"strings"
	"time"
)

type Store struct {
	pool  *pgxpool.Pool
	queue *river.Client[pgx.Tx]
}

// New opens a bounded pool and verifies connectivity. It neither migrates nor
// starts a worker. Connection errors intentionally omit the supplied DSN.
func New(ctx context.Context, dsn string) (*Store, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, errors.New("invalid database configuration")
	}
	cfg.MaxConns = 4
	cfg.MinConns = 0
	cfg.MaxConnLifetime = 30 * time.Minute
	cfg.MaxConnIdleTime = 5 * time.Minute
	cfg.ConnConfig.ConnectTimeout = 5 * time.Second
	p, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, errors.New("database initialization failed")
	}
	if err = p.Ping(ctx); err != nil {
		p.Close()
		return nil, errors.New("database connection unavailable")
	}
	queue, err := river.NewClient(riverpgxv5.New(p), &river.Config{SkipUnknownJobCheck: true})
	if err != nil {
		p.Close()
		return nil, fmt.Errorf("initialize job queue: %w", err)
	}
	return &Store{pool: p, queue: queue}, nil
}
func (s *Store) Close()                         { s.pool.Close() }
func (s *Store) Ping(ctx context.Context) error { return s.pool.Ping(ctx) }

const jobColumns = `id::text,name,state,phase,config,priority,created_at,started_at,finished_at,error,result_count,camera_count,stream_count,inconclusive_count,cancel_requested,attempt,retry_of::text`

func scanJob(row pgx.Row) (JobDTO, error) {
	var j JobDTO
	err := row.Scan(&j.ID, &j.Name, &j.State, &j.Phase, &j.Config, &j.Priority, &j.CreatedAt, &j.StartedAt, &j.FinishedAt, &j.Error, &j.ResultCount, &j.CameraCount, &j.StreamCount, &j.InconclusiveCount, &j.CancelRequested, &j.Attempt, &j.RetryOf)
	if errors.Is(err, pgx.ErrNoRows) {
		return j, ErrNotFound
	}
	return j, err
}

// CreateJob validates under caller-supplied trusted policy and atomically commits
// the immutable job snapshot plus one River job (MaxAttempts=1). A transaction
// advisory lock serializes equal idempotency keys, including concurrent callers.
func (s *Store) CreateJob(ctx context.Context, in CreateJobInput) (JobDTO, error) {
	if in.Priority < 1 || in.Priority > 4 || strings.TrimSpace(in.Name) == "" || len(in.Name) > 200 || len(in.IdempotencyKey) > 200 {
		return JobDTO{}, ErrInvalidInput
	}
	preview, err := config.Validate(ctx, in.Config, in.Policy)
	if err != nil {
		return JobDTO{}, fmt.Errorf("validate config: %w", err)
	}
	in.Config = preview.Config
	if in.RetryOf != nil {
		if _, err := uuid.Parse(*in.RetryOf); err != nil {
			return JobDTO{}, ErrInvalidInput
		}
	}
	// Hash normalized persisted intent, not caller policy or an incidental UUID.
	payload, err := json.Marshal(struct {
		Name        string
		Config      config.ScanConfig
		Priority    int
		ScheduledAt *time.Time
		RetryOf     *string
	}{in.Name, in.Config, in.Priority, in.QueueScheduledAt, in.RetryOf})
	if err != nil {
		return JobDTO{}, ErrInvalidInput
	}
	hash := sha256.Sum256(payload)
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return JobDTO{}, err
	}
	defer tx.Rollback(ctx)
	if in.IdempotencyKey != "" {
		if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,73925))`, in.IdempotencyKey); err != nil {
			return JobDTO{}, err
		}
		var oldHash []byte
		var id string
		err = tx.QueryRow(ctx, `SELECT id::text,request_hash FROM jobs WHERE idempotency_key=$1`, in.IdempotencyKey).Scan(&id, &oldHash)
		if err == nil {
			if !bytes.Equal(oldHash, hash[:]) {
				return JobDTO{}, &ConflictError{}
			}
			return scanJob(tx.QueryRow(ctx, `SELECT `+jobColumns+` FROM jobs WHERE id=$1`, id))
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return JobDTO{}, err
		}
	}
	if in.RetryOf != nil {
		var terminal bool
		err = tx.QueryRow(ctx, `SELECT state IN ('completed','inconclusive','failed','canceled','timed_out','orphaned') FROM jobs WHERE id=$1 FOR SHARE`, *in.RetryOf).Scan(&terminal)
		if errors.Is(err, pgx.ErrNoRows) || err == nil && !terminal {
			return JobDTO{}, ErrInvalidInput
		}
		if err != nil {
			return JobDTO{}, err
		}
	}
	if err = validateReferences(ctx, tx, in.Config); err != nil {
		return JobDTO{}, err
	}
	id := uuid.NewString()
	queued, err := s.queue.InsertTx(ctx, tx, ScanJobArgs{JobID: id}, &river.InsertOpts{MaxAttempts: 1, Priority: in.Priority, ScheduledAt: scheduledAt(in.QueueScheduledAt)})
	if err != nil {
		return JobDTO{}, fmt.Errorf("enqueue scan: %w", err)
	}
	cfg, err := json.Marshal(in.Config)
	if err != nil {
		return JobDTO{}, err
	}
	var key *string
	if in.IdempotencyKey != "" {
		key = &in.IdempotencyKey
	}
	j, err := scanJob(tx.QueryRow(ctx, `INSERT INTO jobs(id,name,config,priority,idempotency_key,request_hash,river_job_id,retry_of) VALUES($1,$2,$3,$4,$5,$6,$7,$8) RETURNING `+jobColumns, id, in.Name, cfg, in.Priority, key, hash[:], queued.Job.ID, in.RetryOf))
	if err != nil {
		return JobDTO{}, fmt.Errorf("persist scan: %w", err)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO job_resource_refs(job_id,proxy_version_id,credential_set_id) VALUES($1,NULLIF($2,'')::uuid,NULLIF($3,'')::uuid)`, id, in.Config.ProxyVersionID, in.Config.CredentialSetID); err != nil {
		return JobDTO{}, err
	}
	if err = audit(ctx, tx, "job.create", id); err != nil {
		return JobDTO{}, err
	}
	if _, err = s.AddEventTx(ctx, tx, id, "state", json.RawMessage(`{"state":"queued"}`)); err != nil {
		return JobDTO{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return JobDTO{}, err
	}
	return j, nil
}
func scheduledAt(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}
	return t.UTC()
}

func (s *Store) GetJob(ctx context.Context, id string) (JobDTO, error) {
	if _, err := uuid.Parse(id); err != nil {
		return JobDTO{}, ErrNotFound
	}
	return scanJob(s.pool.QueryRow(ctx, `SELECT `+jobColumns+` FROM jobs WHERE id=$1`, id))
}
func validState(state string) bool {
	switch state {
	case "", "queued", "running", "canceling", "completed", "inconclusive", "failed", "canceled", "timed_out", "orphaned":
		return true
	}
	return false
}
func (s *Store) ListJobs(ctx context.Context, in ListJobsInput) ([]JobDTO, error) {
	out := make([]JobDTO, 0)
	if !validState(in.State) || in.Offset < 0 || in.Offset > 100000 || len(in.Search) > 200 || in.Limit < 0 || in.Limit > 200 {
		return out, ErrInvalidInput
	}
	if in.Limit == 0 {
		in.Limit = 50
	}
	rows, err := s.pool.Query(ctx, `SELECT `+jobColumns+` FROM jobs WHERE ($1='' OR state=$1) AND ($2='' OR strpos(lower(name),lower($2))>0 OR id::text=$2) ORDER BY created_at DESC,id DESC LIMIT $3 OFFSET $4`, in.State, in.Search, in.Limit, in.Offset)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		j, err := scanJob(rows)
		if err != nil {
			return out, err
		}
		out = append(out, j)
	}
	return out, rows.Err()
}

// CancelQueued uses River's transactional cancellation before locking the app
// row, matching the worker's queue-first locking order. Running jobs receive a
// durable cancellation request; only proven queued River jobs become terminal.
func (s *Store) CancelQueued(ctx context.Context, id string) (JobDTO, error) {
	if _, err := uuid.Parse(id); err != nil {
		return JobDTO{}, ErrNotFound
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return JobDTO{}, err
	}
	defer tx.Rollback(ctx)
	var queueID *int64
	if err = tx.QueryRow(ctx, `SELECT river_job_id FROM jobs WHERE id=$1`, id).Scan(&queueID); errors.Is(err, pgx.ErrNoRows) {
		return JobDTO{}, ErrNotFound
	} else if err != nil {
		return JobDTO{}, err
	}
	var cancelled bool
	if queueID != nil {
		q, err := s.queue.JobCancelTx(ctx, tx, *queueID)
		if err != nil {
			return JobDTO{}, err
		}
		cancelled = q.State == rivertype.JobStateCancelled
	}
	j, err := scanJob(tx.QueryRow(ctx, `SELECT `+jobColumns+` FROM jobs WHERE id=$1 FOR UPDATE`, id))
	if err != nil {
		return JobDTO{}, err
	}
	if j.State == "queued" || j.State == "running" || j.State == "canceling" {
		state := "canceling"
		var finished *time.Time
		if j.State == "queued" && cancelled {
			state = "canceled"
			now := time.Now().UTC()
			finished = &now
		}
		// The app row is locked after River's row: a concurrent claim cannot
		// accidentally become a terminal queued cancellation. Only an actual
		// public state/request change allocates a replay sequence.
		if j.State != state || !j.CancelRequested {
			j, err = scanJob(tx.QueryRow(ctx, `UPDATE jobs SET cancel_requested=true,state=$2,finished_at=COALESCE($3,finished_at),phase=CASE WHEN $2='canceled' THEN 'canceled' ELSE phase END WHERE id=$1 RETURNING `+jobColumns, id, state, finished))
			if err != nil {
				return JobDTO{}, err
			}
			data, err := json.Marshal(struct {
				State           string `json:"state"`
				CancelRequested bool   `json:"cancel_requested"`
			}{j.State, j.CancelRequested})
			if err != nil {
				return JobDTO{}, err
			}
			if _, err = s.AddEventTx(ctx, tx, id, "state", data); err != nil {
				return JobDTO{}, err
			}
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return JobDTO{}, err
	}
	return j, nil
}

// AddEventTx is the narrow transaction helper for worker state+event atomicity.
// Only trusted internal worker code should supply transactions; HTTP never gets
// a pool or a general SQL execution API. Data must be redacted before calling.
func (s *Store) AddEventTx(ctx context.Context, tx pgx.Tx, id, kind string, data json.RawMessage) (Event, error) {
	return addEvent(ctx, tx, id, kind, data)
}

type eventQuerier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func addEvent(ctx context.Context, q eventQuerier, id, kind string, data json.RawMessage) (Event, error) {
	var e Event
	if _, err := uuid.Parse(id); err != nil || len(kind) == 0 || len(kind) > 100 || len(data) > 65536 || !json.Valid(data) {
		return e, ErrInvalidInput
	}
	// Serialize per-job event allocation through commit: sequence order is also
	// publication order, so polling cannot lose a late-committing lower sequence.
	err := q.QueryRow(ctx, `WITH locked AS (SELECT id FROM jobs WHERE id=$1 FOR UPDATE) INSERT INTO job_events(job_id,kind,data) SELECT id,$2,$3 FROM locked RETURNING seq,job_id::text,kind,data,created_at`, id, kind, data).Scan(&e.Seq, &e.JobID, &e.Kind, &e.Data, &e.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return e, ErrNotFound
	}
	return e, err
}
func (s *Store) AddEvent(ctx context.Context, id, kind string, data json.RawMessage) (Event, error) {
	return addEvent(ctx, s.pool, id, kind, data)
}

// GetEvents returns a bounded ascending page strictly after afterSeq. Seq JSON
// is a decimal string to preserve PostgreSQL bigint precision in JavaScript.
func (s *Store) GetEvents(ctx context.Context, id string, afterSeq int64, limit int) ([]Event, error) {
	out := make([]Event, 0)
	if _, err := uuid.Parse(id); err != nil || afterSeq < 0 || limit < 0 || limit > 500 {
		return out, ErrInvalidInput
	}
	if limit == 0 {
		limit = 100
	}
	rows, err := s.pool.Query(ctx, `SELECT seq,job_id::text,kind,data,created_at FROM job_events WHERE job_id=$1 AND seq>$2 ORDER BY seq LIMIT $3`, id, afterSeq, limit)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var e Event
		if err = rows.Scan(&e.Seq, &e.JobID, &e.Kind, &e.Data, &e.CreatedAt); err != nil {
			return out, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
