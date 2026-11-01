package store

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
)

// ResourcePage returns only bounded, explicitly projected public metadata.
func (s *Store) ResourcePage(ctx context.Context, kind, search string, limit, offset int) ([]json.RawMessage, int64, error) {
	out := make([]json.RawMessage, 0)
	q, ok := resourceQueries[kind]
	if !ok || limit < 1 || limit > 200 || offset < 0 || offset > 100000 || len(search) > 200 {
		return out, 0, ErrInvalidInput
	}
	tx, e := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if e != nil {
		return out, 0, e
	}
	defer tx.Rollback(ctx)
	base := `WITH metadata(value) AS (` + q + `) `
	filter := ` WHERE ($1='' OR strpos(lower(COALESCE(value->>'name','')),lower($1))>0 OR value->>'id'=$1)`
	var n int64
	if e = tx.QueryRow(ctx, base+`SELECT count(*) FROM metadata`+filter, search).Scan(&n); e != nil {
		return out, 0, e
	}
	sort := `value->>'created_at' DESC NULLS LAST,value->>'id' DESC`
	if kind == "runners" {
		sort = `value->>'last_seen_at' DESC NULLS LAST,value->>'id' DESC`
	}
	rows, e := tx.Query(ctx, base+`SELECT value FROM metadata`+filter+` ORDER BY `+sort+` LIMIT $2 OFFSET $3`, search, limit, offset)
	if e != nil {
		return out, 0, e
	}
	for rows.Next() {
		var b json.RawMessage
		if e = rows.Scan(&b); e != nil {
			rows.Close()
			return out, 0, e
		}
		out = append(out, b)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return out, 0, e
	}
	if e = tx.Commit(ctx); e != nil {
		return out, 0, e
	}
	return out, n, nil
}
func (s *Store) Resource(ctx context.Context, kind, id string) (json.RawMessage, error) {
	q, ok := resourceQueries[kind]
	if !ok {
		return nil, ErrInvalidInput
	}
	var out json.RawMessage
	e := s.pool.QueryRow(ctx, `WITH metadata(value) AS (`+q+`) SELECT value FROM metadata WHERE value->>'id'=$1`, id).Scan(&out)
	if errors.Is(e, pgx.ErrNoRows) {
		e = ErrNotFound
	}
	return out, e
}
