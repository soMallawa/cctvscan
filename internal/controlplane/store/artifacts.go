package store

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"path/filepath"
	"strings"
)

type ArtifactMetadata struct {
	ID        string `json:"id"`
	MediaType string `json:"media_type"`
	Size      int64  `json:"size_bytes"`
	Digest    string `json:"sha256"`
}
type ArtifactInput struct {
	RelativePath, MediaType, Digest string
	Size                            int64
}

func (s *Store) ArtifactPage(ctx context.Context, id string, limit, offset int) ([]ArtifactMetadata, int64, error) {
	out := make([]ArtifactMetadata, 0)
	if limit < 1 || limit > 200 || offset < 0 || offset > 100000 {
		return out, 0, ErrInvalidInput
	}
	tx, e := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if e != nil {
		return out, 0, e
	}
	defer tx.Rollback(ctx)
	var count int64
	if e = tx.QueryRow(ctx, `SELECT count(*) FROM artifacts WHERE job_id=$1`, id).Scan(&count); e != nil {
		return out, 0, e
	}
	rows, e := tx.Query(ctx, `SELECT id::text,media_type,size_bytes,sha256 FROM artifacts WHERE job_id=$1 ORDER BY id LIMIT $2 OFFSET $3`, id, limit, offset)
	if e != nil {
		return out, 0, e
	}
	for rows.Next() {
		var a ArtifactMetadata
		if e = rows.Scan(&a.ID, &a.MediaType, &a.Size, &a.Digest); e != nil {
			rows.Close()
			return out, 0, e
		}
		out = append(out, a)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return out, 0, e
	}
	if e = tx.Commit(ctx); e != nil {
		return out, 0, e
	}
	return out, count, nil
}
func (s *Store) SaveArtifacts(ctx context.Context, a Attempt, artifacts []ArtifactInput) error {
	if len(artifacts) > 50 {
		return ErrInvalidInput
	}
	tx, e := s.pool.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	var current bool
	if e = tx.QueryRow(ctx, `SELECT j.state IN ('running','canceling') AND EXISTS(SELECT 1 FROM attempts a WHERE a.id=$2 AND a.job_id=j.id AND a.fencing_token=$3 AND a.number=j.attempt AND a.state='running') FROM jobs j WHERE j.id=$1 FOR UPDATE`, a.JobID, a.ID, a.Token).Scan(&current); e != nil {
		return e
	}
	if !current {
		return ErrFenced
	}
	added := int64(0)
	for _, artifact := range artifacts {
		decoded, e := hex.DecodeString(artifact.Digest)
		if e != nil || len(decoded) != 32 {
			return ErrInvalidInput
		}
		prefix := a.JobID + "/" + a.ID + "/"
		path := filepath.ToSlash(artifact.RelativePath)
		if !strings.HasPrefix(path, prefix) || filepath.ToSlash(filepath.Clean(path)) != path || strings.Contains(path, "\\") || artifact.Size < 1 || artifact.Size > 5<<20 || (artifact.MediaType != "image/jpeg" && artifact.MediaType != "image/png") {
			return ErrInvalidInput
		}
		tag, e := tx.Exec(ctx, `INSERT INTO artifacts(id,job_id,attempt_id,relative_path,media_type,size_bytes,sha256) VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT(job_id,relative_path) DO NOTHING`, uuid.NewString(), a.JobID, a.ID, path, artifact.MediaType, artifact.Size, artifact.Digest)
		if e != nil {
			return e
		}
		added += tag.RowsAffected()
	}
	if added > 0 {
		data, _ := json.Marshal(map[string]int64{"stored_images": added})
		if _, e = s.AddEventTx(ctx, tx, a.JobID, "artifact", data); e != nil {
			return e
		}
	}
	return tx.Commit(ctx)
}
