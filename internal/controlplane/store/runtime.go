package store

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/postfix/cctvscan/internal/controlplane/config"
)

var ErrCapacity = errors.New("runner capacity unavailable")
var ErrFenced = errors.New("attempt is no longer current")

type Attempt struct {
	ID, JobID, Token, RunnerID string
	Job                        JobDTO
}

func (s *Store) RegisterRunner(ctx context.Context, id string) error {
	if _, e := uuid.Parse(id); e != nil {
		return ErrInvalidInput
	}
	_, e := s.pool.Exec(ctx, `INSERT INTO runners(id,capabilities,slots,stats) VALUES($1,'["connect"]',1,'{"name":"CONNECT runner"}')`, id)
	return e
}

// BeginAttempt serializes the installation budget before reserving a job. An
// existing attempt is never resumed or silently executed a second time.
func (s *Store) BeginAttempt(ctx context.Context, id, runner string) (Attempt, error) {
	var a Attempt
	tx, e := s.pool.Begin(ctx)
	if e != nil {
		return a, e
	}
	defer tx.Rollback(ctx)
	var raw json.RawMessage
	var capacity int
	if e = tx.QueryRow(ctx, `SELECT policy,concurrency FROM settings WHERE singleton FOR UPDATE`).Scan(&raw, &capacity); e != nil {
		return a, e
	}
	j, e := scanJob(tx.QueryRow(ctx, `SELECT `+jobColumns+` FROM jobs WHERE id=$1 FOR UPDATE`, id))
	if e != nil {
		return a, e
	}
	if j.Attempt == 0 && j.CancelRequested && (j.State == "queued" || j.State == "canceling") {
		if _, e = tx.Exec(ctx, `UPDATE jobs SET state='canceled',phase='canceled',finished_at=now() WHERE id=$1`, id); e != nil {
			return a, e
		}
		if _, e = s.AddEventTx(ctx, tx, id, "state", json.RawMessage(`{"state":"canceled","cancel_requested":true}`)); e != nil {
			return a, e
		}
		if e = tx.Commit(ctx); e != nil {
			return a, e
		}
		return a, ErrFenced
	}
	if j.State != "queued" || j.Attempt != 0 {
		return a, ErrFenced
	}
	var ready bool
	if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM runners WHERE id=$1 AND slots>0 AND heartbeat_at>now()-interval '30 seconds')`, runner).Scan(&ready); e != nil {
		return a, e
	}
	if !ready {
		return a, ErrCapacity
	}
	var active int
	if e = tx.QueryRow(ctx, `SELECT count(*) FROM attempts WHERE state='running' AND runner_id IS NOT NULL`).Scan(&active); e != nil {
		return a, e
	}
	if active >= capacity {
		return a, ErrCapacity
	}
	policy := config.DefaultPolicy()
	if string(raw) != "{}" {
		if e = json.Unmarshal(raw, &policy); e != nil {
			return a, e
		}
	}
	if _, e = config.Validate(ctx, j.Config, policy); e != nil {
		if _, e = tx.Exec(ctx, `UPDATE jobs SET state='failed',phase='validation',finished_at=now(),error='Current server policy rejects this saved execution request.' WHERE id=$1`, id); e != nil {
			return a, e
		}
		if _, e = s.AddEventTx(ctx, tx, id, "state", json.RawMessage(`{"state":"failed","phase":"validation","message":"Current server policy rejects the saved execution request."}`)); e != nil {
			return a, e
		}
		if e = tx.Commit(ctx); e != nil {
			return a, e
		}
		return a, ErrInvalidInput
	}
	a = Attempt{ID: uuid.NewString(), JobID: id, Token: uuid.NewString(), RunnerID: runner}
	if _, e = tx.Exec(ctx, `INSERT INTO attempts(id,job_id,number,fencing_token,state,runner_id) VALUES($1,$2,1,$3,'running',$4)`, a.ID, id, a.Token, runner); e != nil {
		return a, e
	}
	a.Job, e = scanJob(tx.QueryRow(ctx, `UPDATE jobs SET state='running',phase='preparing',started_at=now(),attempt=1 WHERE id=$1 RETURNING `+jobColumns, id))
	if e != nil {
		return a, e
	}
	if _, e = s.AddEventTx(ctx, tx, id, "state", json.RawMessage(`{"state":"running","phase":"preparing","attempt":1}`)); e != nil {
		return a, e
	}
	if e = tx.Commit(ctx); e != nil {
		return Attempt{}, e
	}
	return a, nil
}

// FinishAttempt requires the current fencing token and preserves a cancellation
// already committed before the stopped child was acknowledged by its runner.
func (s *Store) FinishAttempt(ctx context.Context, a Attempt, state string) error {
	if !Terminal(state) {
		return ErrInvalidInput
	}
	tx, e := s.pool.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	j, e := scanJob(tx.QueryRow(ctx, `SELECT `+jobColumns+` FROM jobs WHERE id=$1 FOR UPDATE`, a.JobID))
	if e != nil {
		return e
	}
	if j.State != "running" && j.State != "canceling" {
		return ErrFenced
	}
	if j.CancelRequested {
		state = "canceled"
	}
	tag, e := tx.Exec(ctx, `UPDATE attempts SET state=$1,finished_at=now() WHERE id=$2 AND job_id=$3 AND fencing_token=$4 AND state='running' AND number=$5`, state, a.ID, a.JobID, a.Token, j.Attempt)
	if e != nil {
		return e
	}
	if tag.RowsAffected() != 1 {
		return ErrFenced
	}
	message := ""
	switch state {
	case "failed":
		message = "Scanner execution or report import failed."
	case "inconclusive":
		message = "Execution retained incomplete evidence."
	case "timed_out":
		message = "The execution deadline expired."
	case "orphaned":
		message = "Runner execution was interrupted; coverage is uncertain."
	}
	if _, e = tx.Exec(ctx, `UPDATE jobs SET state=$1,phase=$1,finished_at=now(),error=$2 WHERE id=$3`, state, message, a.JobID); e != nil {
		return e
	}
	data, _ := json.Marshal(map[string]any{"state": state, "cancel_requested": j.CancelRequested})
	if _, e = s.AddEventTx(ctx, tx, a.JobID, "state", data); e != nil {
		return e
	}
	return tx.Commit(ctx)
}

// ActiveAttempt checks both liveness and durable cancellation. Database failure
// is an execution stop condition, never permission to continue without a lease.
func (s *Store) ActiveAttempt(ctx context.Context, a Attempt) (bool, error) {
	var cancel bool
	e := s.pool.QueryRow(ctx, `SELECT j.cancel_requested FROM attempts a JOIN jobs j ON j.id=a.job_id JOIN runners r ON r.id=a.runner_id WHERE a.id=$1 AND a.job_id=$2 AND a.fencing_token=$3 AND a.state='running' AND a.number=j.attempt AND j.state IN ('running','canceling') AND r.heartbeat_at>now()-interval '30 seconds'`, a.ID, a.JobID, a.Token).Scan(&cancel)
	if errors.Is(e, pgx.ErrNoRows) {
		e = ErrFenced
	}
	return cancel, e
}
