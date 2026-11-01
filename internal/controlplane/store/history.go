package store

import (
	"context"
	"strconv"
)

func (s *Store) EventCursor(ctx context.Context) (string, error) {
	var n int64
	e := s.pool.QueryRow(ctx, `SELECT COALESCE(max(seq),0) FROM job_events`).Scan(&n)
	return strconv.FormatInt(n, 10), e
}
func (s *Store) EventHistory(ctx context.Context, id string, before int64, limit int) ([]Event, bool, error) {
	out := make([]Event, 0)
	if before < 0 || limit < 1 || limit > 200 {
		return out, false, ErrInvalidInput
	}
	rows, e := s.pool.Query(ctx, `SELECT seq,job_id::text,kind,data,created_at FROM job_events WHERE job_id=$1 AND ($2::bigint=0 OR seq<$2) ORDER BY seq DESC LIMIT $3`, id, before, limit+1)
	if e != nil {
		return out, false, e
	}
	defer rows.Close()
	for rows.Next() {
		var v Event
		if e = rows.Scan(&v.Seq, &v.JobID, &v.Kind, &v.Data, &v.CreatedAt); e != nil {
			return out, false, e
		}
		out = append(out, v)
	}
	if e = rows.Err(); e != nil {
		return out, false, e
	}
	more := len(out) > limit
	if more {
		out = out[:limit]
	}
	return out, more, nil
}
