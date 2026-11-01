package store

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/postfix/cctvscan/internal/connection"
	"github.com/postfix/cctvscan/internal/controlplane/config"
	"time"
)

type ResultEvidence struct {
	ID     int64
	Target string
	Data   json.RawMessage
	Config config.ScanConfig
}

func (s *Store) ResultEvidence(ctx context.Context, id int64) (ResultEvidence, error) {
	var r ResultEvidence
	e := s.pool.QueryRow(ctx, `SELECT r.id,r.target,r.data,j.config FROM results r JOIN jobs j ON j.id=r.job_id WHERE r.id=$1`, id).Scan(&r.ID, &r.Target, &r.Data, &r.Config)
	if errors.Is(e, pgx.ErrNoRows) {
		e = ErrNotFound
	}
	return r, e
}

// ConnectionProxy is trusted server-only ciphertext lookup, bound to an immutable job.
func (s *Store) ConnectionProxy(ctx context.Context, version string) ([]byte, error) {
	var sealed []byte
	e := s.pool.QueryRow(ctx, `SELECT sealed_url FROM proxy_versions WHERE id=$1`, version).Scan(&sealed)
	return sealed, e
}

type ConnectionCheck struct {
	ID        string             `json:"id"`
	ResultID  int64              `json:"result_id"`
	ProfileID string             `json:"profile_id"`
	Outcome   connection.Outcome `json:"outcome"`
	CreatedAt time.Time          `json:"created_at"`
}

func (s *Store) SaveConnectionCheck(ctx context.Context, result int64, profile string, out connection.Outcome) (ConnectionCheck, error) {
	v := ConnectionCheck{ID: uuid.NewString(), ResultID: result, ProfileID: profile, Outcome: out}
	raw, e := json.Marshal(out)
	if e != nil || len(raw) > 1048576 {
		return v, ErrInvalidInput
	}
	e = s.pool.QueryRow(ctx, `INSERT INTO connection_checks(id,result_id,profile_id,outcome) VALUES($1,$2,$3,$4) RETURNING created_at`, v.ID, result, profile, raw).Scan(&v.CreatedAt)
	return v, e
}
func (s *Store) ConnectionChecks(ctx context.Context, result int64) ([]ConnectionCheck, error) {
	out := []ConnectionCheck{}
	rows, e := s.pool.Query(ctx, `SELECT id::text,result_id,profile_id,outcome,created_at FROM connection_checks WHERE result_id=$1 ORDER BY created_at DESC,id DESC LIMIT 20`, result)
	if e != nil {
		return out, e
	}
	defer rows.Close()
	for rows.Next() {
		var v ConnectionCheck
		if e = rows.Scan(&v.ID, &v.ResultID, &v.ProfileID, &v.Outcome, &v.CreatedAt); e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
