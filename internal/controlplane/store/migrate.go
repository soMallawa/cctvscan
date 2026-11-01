package store

import (
	"context"
	"crypto/sha256"
	"embed"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"
	"sort"
	"time"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

// Migrate serializes River and application migrations under a session advisory
// lock. River needs separate commits for enum changes; application migrations
// and their checksums are transactional. Never call this per request.
func (s *Store) Migrate(ctx context.Context) error {
	// Do not occupy pool slots while waiting for another migrator: the lock
	// holder needs a second connection for River's per-version transactions.
	var conn *pgxpool.Conn
	for {
	 var err error
	 conn,err=s.pool.Acquire(ctx)
	 if err!=nil{return fmt.Errorf("acquire migration connection: %w",err)}
	 var locked bool
	 if err=conn.QueryRow(ctx,`SELECT pg_try_advisory_lock(739250011)`).Scan(&locked);err!=nil{conn.Release();return fmt.Errorf("lock migration: %w",err)}
	 if locked{break}
	 conn.Release()
	 timer:=time.NewTimer(25*time.Millisecond)
	 select{case <-ctx.Done():timer.Stop();return ctx.Err();case <-timer.C:}
	}
	defer conn.Release()
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := conn.Exec(cleanup, `SELECT pg_advisory_unlock(739250011)`); err != nil {
			_ = conn.Hijack().Close(cleanup)
		}
	}()
	migrator, err := rivermigrate.New(riverpgxv5.New(s.pool), nil)
	if err != nil {
		return fmt.Errorf("river migrator: %w", err)
	}
	if _, err = migrator.Migrate(ctx, rivermigrate.DirectionUp, nil); err != nil {
		return fmt.Errorf("river migrate: %w", err)
	}
	tx, err := conn.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin migration: %w", err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `CREATE TABLE IF NOT EXISTS controlplane_migrations(version text PRIMARY KEY, checksum bytea NOT NULL, applied_at timestamptz NOT NULL DEFAULT now())`); err != nil {
		return fmt.Errorf("migration ledger: %w", err)
	}
	entries, err := migrationFS.ReadDir("migrations")
	if err != nil {
		return err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, entry := range entries {
		name := entry.Name()
		sql, err := migrationFS.ReadFile("migrations/" + name)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(sql)
		var applied bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM controlplane_migrations WHERE version=$1)`, name).Scan(&applied); err != nil {
			return err
		}
		if applied {
			var matches bool
			if err = tx.QueryRow(ctx, `SELECT checksum=$2 FROM controlplane_migrations WHERE version=$1`, name, sum[:]).Scan(&matches); err != nil {
				return err
			}
			if !matches {
				return fmt.Errorf("migration checksum mismatch: %s", name)
			}
			continue
		}
		if _, err = tx.Exec(ctx, string(sql)); err != nil {
			return fmt.Errorf("apply %s: %w", name, err)
		}
		if _, err = tx.Exec(ctx, `INSERT INTO controlplane_migrations(version,checksum) VALUES($1,$2)`, name, sum[:]); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
