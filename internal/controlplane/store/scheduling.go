package store

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/postfix/cctvscan/internal/controlplane/config"
	"github.com/riverqueue/river"
	"github.com/robfig/cron/v3"
	"time"
)

// DispatchSchedules handles a bounded set of due occurrences. The schedule row,
// occurrence ledger, job snapshot, resource pins, River delivery and replayable
// event commit together. Concurrent scheduler loops cannot publish duplicates.
func (s *Store) DispatchSchedules(ctx context.Context, now time.Time) (int, error) {
	rows, e := s.pool.Query(ctx, `SELECT id::text FROM schedules WHERE enabled AND next_run_at<=$1 ORDER BY next_run_at,id LIMIT 25`, now)
	if e != nil {
		return 0, e
	}
	var ids []string
	for rows.Next() {
		var id string
		if e = rows.Scan(&id); e != nil {
			rows.Close()
			return 0, e
		}
		ids = append(ids, id)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return 0, e
	}
	count := 0
	for _, id := range ids {
		n, e := s.dispatchSchedule(ctx, id, now)
		if e != nil {
			return count, e
		}
		count += n
	}
	return count, nil
}
func (s *Store) dispatchSchedule(ctx context.Context, id string, now time.Time) (int, error) {
	tx, e := s.pool.Begin(ctx)
	if e != nil {
		return 0, e
	}
	defer tx.Rollback(ctx)
	var name, expr, zone, misfire, overlap string
	var raw json.RawMessage
	var due time.Time
	e = tx.QueryRow(ctx, `SELECT name,config,cron,timezone,misfire_policy,overlap_policy,next_run_at FROM schedules WHERE id=$1 AND enabled AND next_run_at<=$2 FOR UPDATE SKIP LOCKED`, id, now).Scan(&name, &raw, &expr, &zone, &misfire, &overlap, &due)
	if errors.Is(e, pgx.ErrNoRows) {
		return 0, nil
	}
	if e != nil {
		return 0, e
	}
	var next *time.Time
	state := "queued"
	message := ""
	location, e := time.LoadLocation(zone)
	if e == nil {
		var schedule cron.Schedule
		schedule, e = cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow).Parse(expr)
		if e == nil {
			n := schedule.Next(now.In(location))
			if !n.IsZero() {
				n = n.UTC()
				next = &n
			}
		}
	}
	if e != nil || next == nil {
		state = "rejected"
		message = "Schedule expression or timezone is no longer valid."
	}
	var exists bool
	if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM schedule_occurrences WHERE schedule_id=$1 AND scheduled_at=$2)`, id, due).Scan(&exists); e != nil {
		return 0, e
	}
	if exists {
		if _, e = tx.Exec(ctx, `UPDATE schedules SET next_run_at=$2,enabled=($2::timestamptz IS NOT NULL) WHERE id=$1`, id, next); e != nil {
			return 0, e
		}
		return 0, tx.Commit(ctx)
	}
	if state == "queued" && misfire == "skip" && now.Sub(due) >= time.Minute {
		state = "misfire_skipped"
		message = "Missed occurrence skipped; no backlog was replayed."
	}
	if state == "queued" && overlap == "skip" {
		var active bool
		e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM schedule_occurrences o JOIN jobs j ON j.id=o.job_id WHERE o.schedule_id=$1 AND j.state IN ('queued','running','canceling'))`, id).Scan(&active)
		if e != nil {
			return 0, e
		}
		if active {
			state = "overlap_skipped"
			message = "Previous occurrence is still queued or active."
		}
	}
	var c config.ScanConfig
	var policyRaw json.RawMessage
	policy := config.DefaultPolicy()
	if state == "queued" {
		if e = tx.QueryRow(ctx, `SELECT policy FROM settings WHERE singleton`).Scan(&policyRaw); e != nil {
			return 0, e
		}
		if string(policyRaw) != "{}" {
			if e = json.Unmarshal(policyRaw, &policy); e != nil {
				return 0, e
			}
		}
		e = json.Unmarshal(raw, &c)
		if e == nil {
			_, e = config.Validate(ctx, c, policy)
		}
		if e == nil {
			e = validateReferences(ctx, tx, c)
		}
		if e != nil {
			state = "rejected"
			message = "Current policy or resource availability rejects this schedule snapshot."
			next = nil
		}
	}
	var jobID *string
	if state == "queued" {
		created := uuid.NewString()
		jobID = &created
		hash := sha256.Sum256(raw)
		if _, e = tx.Exec(ctx, `INSERT INTO jobs(id,name,config,state,priority,request_hash) VALUES($1,$2,$3,'queued',2,$4)`, created, name, raw, fmt.Sprintf("%x", hash)); e != nil {
			return 0, e
		}
		if _, e = tx.Exec(ctx, `INSERT INTO job_resource_refs(job_id,proxy_version_id,credential_set_id) VALUES($1,NULLIF($2,'')::uuid,NULLIF($3,'')::uuid)`, created, c.ProxyVersionID, c.CredentialSetID); e != nil {
			return 0, e
		}
		queued, e := s.queue.InsertTx(ctx, tx, ScanJobArgs{JobID: created}, &river.InsertOpts{MaxAttempts: 1, Priority: 2})
		if e != nil {
			return 0, e
		}
		if _, e = tx.Exec(ctx, `UPDATE jobs SET river_job_id=$2 WHERE id=$1`, created, queued.Job.ID); e != nil {
			return 0, e
		}
		data, _ := json.Marshal(map[string]string{"state": "queued", "schedule_id": id, "scheduled_at": due.UTC().Format(time.RFC3339Nano)})
		if _, e = s.AddEventTx(ctx, tx, created, "state", data); e != nil {
			return 0, e
		}
	}
	if _, e = tx.Exec(ctx, `INSERT INTO schedule_occurrences(schedule_id,scheduled_at,job_id,state,message) VALUES($1,$2,$3,$4,$5)`, id, due, jobID, state, message); e != nil {
		return 0, e
	}
	if _, e = tx.Exec(ctx, `UPDATE schedules SET last_run_at=$2,next_run_at=$3,enabled=($3::timestamptz IS NOT NULL) WHERE id=$1`, id, now, next); e != nil {
		return 0, e
	}
	metadata, _ := json.Marshal(map[string]any{"scheduled_at": due, "state": state, "job_id": jobID})
	if _, e = tx.Exec(ctx, `INSERT INTO audit_events(actor,action,resource_id,metadata) VALUES('scheduler','schedule.occurrence',$1,$2)`, id, metadata); e != nil {
		return 0, e
	}
	if e = tx.Commit(ctx); e != nil {
		return 0, e
	}
	if jobID != nil {
		return 1, nil
	}
	return 0, nil
}
