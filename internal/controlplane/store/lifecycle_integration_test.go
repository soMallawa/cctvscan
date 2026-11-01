//go:build integration

package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestLifecycleStates(t *testing.T) {
	ctx := context.Background()
	s, _ := fixture(t)
	for _, state := range []string{"queued", "running", "canceling", "completed", "inconclusive", "failed", "canceled", "timed_out", "orphaned"} {
		t.Run(state, func(t *testing.T) {
			in := input()
			in.Name = "lifecycle-" + uuid.NewString()
			j, err := s.CreateJob(ctx, in)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = s.pool.Exec(ctx, `UPDATE jobs SET state=$2 WHERE id=$1`, j.ID, state); err != nil {
				t.Fatal("persist job state:", err)
			}
			attemptID := uuid.NewString()
			if _, err = s.pool.Exec(ctx, `INSERT INTO attempts(id,job_id,number,fencing_token,state) VALUES($1,$2,1,$3,$4)`, attemptID, j.ID, uuid.NewString(), state); err != nil {
				t.Fatal("persist attempt state:", err)
			}
			var attemptState string
			if err = s.pool.QueryRow(ctx, `SELECT state FROM attempts WHERE id=$1`, attemptID).Scan(&attemptState); err != nil || attemptState != state {
				t.Fatalf("attempt roundtrip = %q, %v", attemptState, err)
			}
			got, err := s.GetJob(ctx, j.ID)
			if err != nil || got.State != state {
				t.Fatalf("job roundtrip = %q, %v", got.State, err)
			}
			listed, err := s.ListJobs(ctx, ListJobsInput{State: state, Search: in.Name})
			if err != nil || len(listed) != 1 || listed[0].ID != j.ID || listed[0].State != state {
				t.Fatalf("filtered listing = %+v, %v", listed, err)
			}
			retry := input()
			retry.RetryOf = &j.ID
			retried, err := s.CreateJob(ctx, retry)
			terminal := state != "queued" && state != "running" && state != "canceling"
			if terminal {
				if err != nil || retried.RetryOf == nil || *retried.RetryOf != j.ID || retried.State != "queued" {
					t.Fatalf("terminal retry = %+v, %v", retried, err)
				}
				canceled, err := s.CancelQueued(ctx, j.ID)
				if err != nil || canceled.State != state || canceled.CancelRequested {
					t.Fatalf("terminal state changed on cancel = %+v, %v", canceled, err)
				}
			} else if !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("active retry error = %v", err)
			}
		})
	}
	for _, state := range []string{"succeeded", "cancelled", "interrupted", "bogus"} {
		if _, err := s.ListJobs(ctx, ListJobsInput{State: state}); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("legacy/invalid filter %q: %v", state, err)
		}
		j, err := s.CreateJob(ctx, input())
		if err != nil {
			t.Fatal(err)
		}
		if _, err = s.pool.Exec(ctx, `UPDATE jobs SET state=$2 WHERE id=$1`, j.ID, state); err == nil {
			t.Fatalf("jobs accepted invalid state %q", state)
		}
		if _, err = s.pool.Exec(ctx, `INSERT INTO attempts(id,job_id,number,fencing_token,state) VALUES($1,$2,1,$3,$4)`, uuid.NewString(), j.ID, uuid.NewString(), state); err == nil {
			t.Fatalf("attempts accepted invalid state %q", state)
		}
	}
}

func TestLifecycleCancellation(t *testing.T) {
	ctx := context.Background()
	s, _ := fixture(t)
	for _, state := range []string{"queued", "running", "canceling"} {
		t.Run(state, func(t *testing.T) {
			j, err := s.CreateJob(ctx, input())
			if err != nil {
				t.Fatal(err)
			}
			if state != "queued" {
				if _, err = s.pool.Exec(ctx, `UPDATE jobs SET state=$2 WHERE id=$1`, j.ID, state); err != nil {
					t.Fatal(err)
				}
				if _, err = s.pool.Exec(ctx, `UPDATE river_job SET state='running',attempted_at=now() WHERE args->>'job_id'=$1`, j.ID); err != nil {
					t.Fatal(err)
				}
			}
			want := "canceling"
			if state == "queued" {
				want = "canceled"
			}
			got, err := s.CancelQueued(ctx, j.ID)
			if err != nil || got.State != want || !got.CancelRequested {
				t.Fatalf("cancel = %+v, %v", got, err)
			}
			if state == "queued" {
				if got.FinishedAt == nil || got.Phase != "canceled" {
					t.Fatal("queued cancellation lacks terminal timestamp/phase")
				}
				var riverState string
				if err = s.pool.QueryRow(ctx, `SELECT state FROM river_job WHERE args->>'job_id'=$1`, j.ID).Scan(&riverState); err != nil || riverState != "cancelled" {
					t.Fatalf("River state = %q, %v", riverState, err)
				}
			} else if got.FinishedAt != nil {
				t.Fatal("running cancellation prematurely terminal")
			}
			again, err := s.CancelQueued(ctx, j.ID)
			if err != nil || again.State != want || !again.CancelRequested {
				t.Fatalf("repeat cancel = %+v, %v", again, err)
			}
		})
	}
}

func TestLifecycleMigrationFrom001(t *testing.T) {
	ctx := context.Background()
	admin, dsn := fixture(t)
	// Isolate legacy application tables; River stays in the disposable fixture's public schema.
	uid := uuid.New()
	schema := "lifecycle_" + hex.EncodeToString(uid[:])
	quoted := pgx.Identifier{schema}.Sanitize()
	if _, err := admin.pool.Exec(ctx, `CREATE SCHEMA `+quoted); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := admin.pool.Exec(ctx, `DROP SCHEMA `+quoted+` CASCADE`); err != nil {
			t.Error(err)
		}
	})
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal("parse test database configuration")
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema + ",public"
	cfg.MaxConns = 4
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal("open isolated fixture")
	}
	defer pool.Close()
	legacy := &Store{pool: pool}
	initial, err := migrationFS.ReadFile("migrations/001_initial.sql")
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(initial)
	if hex.EncodeToString(sum[:]) != "d914887843d491454d77000f229466763e9884617db9605c14b97fe984a49a35" {
		t.Fatal("applied migration 001 was changed")
	}
	if _, err = pool.Exec(ctx, string(initial)); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `CREATE TABLE controlplane_migrations(version text PRIMARY KEY,checksum bytea NOT NULL,applied_at timestamptz NOT NULL DEFAULT now())`); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO controlplane_migrations(version,checksum) VALUES('001_initial.sql',$1)`, sum[:]); err != nil {
		t.Fatal(err)
	}
	ids := map[string]string{}
	for old, want := range map[string]string{"succeeded": "completed", "cancelled": "canceled", "interrupted": "orphaned"} {
		id := uuid.NewString()
		ids[want] = id
		if _, err = pool.Exec(ctx, `INSERT INTO jobs(id,name,state,config,priority,request_hash) VALUES($1,'legacy',$2,'{"targets":["192.0.2.1"]}',2,'legacy-hash')`, id, old); err != nil {
			t.Fatal(err)
		}
		if _, err = pool.Exec(ctx, `INSERT INTO attempts(id,job_id,number,fencing_token,state) VALUES($1,$2,1,$3,$4)`, uuid.NewString(), id, uuid.NewString(), old); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 2; i++ {
		if err = legacy.Migrate(ctx); err != nil {
			t.Fatal(err)
		}
		for want, id := range ids {
			var jobState, attemptState, target, hash string
			if err = pool.QueryRow(ctx, `SELECT j.state,a.state,j.config->'targets'->>0,convert_from(j.request_hash,'UTF8') FROM jobs j JOIN attempts a ON a.job_id=j.id WHERE j.id=$1`, id).Scan(&jobState, &attemptState, &target, &hash); err != nil {
				t.Fatal(err)
			}
			if jobState != want || attemptState != want || target != "192.0.2.1" || hash != "legacy-hash" {
				t.Fatalf("migration round %d: job=%q attempt=%q target=%q hash=%q", i, jobState, attemptState, target, hash)
			}
		}
		entries, err := migrationFS.ReadDir("migrations")
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) < 2 {
			t.Fatalf("want 001 + new 002; got %d migrations", len(entries))
		}
		var count int
		if err = pool.QueryRow(ctx, `SELECT count(*) FROM controlplane_migrations`).Scan(&count); err != nil || count != len(entries) {
			t.Fatalf("migration ledger count = %d, %v", count, err)
		}
		for _, entry := range entries {
			data, err := migrationFS.ReadFile("migrations/" + entry.Name())
			if err != nil {
				t.Fatal(err)
			}
			checksum := sha256.Sum256(data)
			var matches bool
			if err = pool.QueryRow(ctx, `SELECT checksum=$2 FROM controlplane_migrations WHERE version=$1`, entry.Name(), checksum[:]).Scan(&matches); err != nil || !matches {
				t.Fatalf("ledger checksum %s: %v, %v", entry.Name(), matches, err)
			}
		}
	}
	// Existing checksum enforcement must still reject a changed applied migration.
	if _, err = pool.Exec(ctx, `UPDATE controlplane_migrations SET checksum='tampered' WHERE version='001_initial.sql'`); err != nil {
		t.Fatal(err)
	}
	if err = legacy.Migrate(ctx); err == nil {
		t.Fatal("accepted changed applied migration checksum")
	}
}
