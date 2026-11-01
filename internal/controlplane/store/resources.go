package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/postfix/cctvscan/internal/controlplane/config"
	"strings"
	"time"
)

// FieldError contains only server-owned validation text, never driver errors.
type FieldError struct{ Field, Message string }

func (e *FieldError) Error() string { return e.Field + ": " + e.Message }

var ErrReferenced = errors.New("resource is referenced by live jobs or saved configurations")
var ErrNotTerminal = errors.New("only terminal jobs may be retried or deleted")

func audit(ctx context.Context, tx pgx.Tx, action, id string) error {
	_, e := tx.Exec(ctx, `INSERT INTO audit_events(actor,action,resource_id) VALUES('operator',$1,$2)`, action, id)
	return e
}
func validateReferences(ctx context.Context, tx pgx.Tx, c config.ScanConfig) error {
	if c.ProxyVersionID != "" {
		if _, e := uuid.Parse(c.ProxyVersionID); e != nil {
			return &FieldError{"proxy_version_id", "select an existing proxy version"}
		}
		var id string
		e := tx.QueryRow(ctx, `SELECT v.id::text FROM proxy_versions v JOIN proxy_profiles p ON p.id=v.profile_id WHERE v.id=$1 AND p.retired_at IS NULL FOR SHARE OF p,v`, c.ProxyVersionID).Scan(&id)
		if errors.Is(e, pgx.ErrNoRows) {
			return &FieldError{"proxy_version_id", "proxy version is missing or retired"}
		}
		if e != nil {
			return e
		}
	}
	if c.CredentialSetID != "" {
		if _, e := uuid.Parse(c.CredentialSetID); e != nil {
			return &FieldError{"credential_set_id", "select an existing credential set"}
		}
		var id string
		e := tx.QueryRow(ctx, `SELECT id::text FROM credential_sets WHERE id=$1 AND retired_at IS NULL FOR SHARE`, c.CredentialSetID).Scan(&id)
		if errors.Is(e, pgx.ErrNoRows) {
			return &FieldError{"credential_set_id", "credential set is missing or retired"}
		}
		if e != nil {
			return e
		}
	}
	return nil
}
func (s *Store) ValidateReferences(ctx context.Context, c config.ScanConfig) error {
	tx, e := s.pool.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	return validateReferences(ctx, tx, c)
}
func (s *Store) CountJobs(ctx context.Context, in ListJobsInput) (int64, error) {
	var n int64
	e := s.pool.QueryRow(ctx, `SELECT count(*) FROM jobs WHERE ($1='' OR state=$1) AND ($2='' OR strpos(lower(name),lower($2))>0 OR id::text=$2)`, in.State, in.Search).Scan(&n)
	return n, e
}
func (s *Store) DeleteJob(ctx context.Context, id string) error {
	tx, e := s.pool.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	j, e := scanJob(tx.QueryRow(ctx, `SELECT `+jobColumns+` FROM jobs WHERE id=$1 FOR UPDATE`, id))
	if e != nil {
		return e
	}
	if !Terminal(j.State) {
		return ErrNotTerminal
	}
	var referenced bool
	if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM jobs WHERE retry_of=$1) OR EXISTS(SELECT 1 FROM schedule_occurrences WHERE job_id=$1)`, id).Scan(&referenced); e != nil {
		return e
	}
	if referenced {
		return ErrReferenced
	}
	for _, q := range []string{`DELETE FROM job_events WHERE job_id=$1`, `DELETE FROM artifacts WHERE job_id=$1`, `DELETE FROM results WHERE job_id=$1`, `DELETE FROM attempts WHERE job_id=$1`, `DELETE FROM jobs WHERE id=$1`} {
		if _, e = tx.Exec(ctx, q, id); e != nil {
			return e
		}
	}
	if e = audit(ctx, tx, "job.delete", id); e != nil {
		return e
	}
	return tx.Commit(ctx)
}
func Terminal(state string) bool {
	switch state {
	case "completed", "inconclusive", "failed", "canceled", "timed_out", "orphaned":
		return true
	}
	return false
}

// Resource queries are closed over a server-owned kind whitelist. JSON is built
// from explicit metadata columns, so ciphertext can never enter a read DTO.
var resourceQueries = map[string]string{
	"proxies":         `SELECT jsonb_build_object('id',p.id,'version_id',v.id,'name',p.name,'created_at',p.created_at,'last_test_at',p.last_test_at,'last_test_ok',p.last_test_ok,'last_test_message',p.last_test_message)||v.metadata FROM proxy_profiles p JOIN LATERAL (SELECT id,metadata FROM proxy_versions WHERE profile_id=p.id ORDER BY version DESC LIMIT 1) v ON true WHERE p.retired_at IS NULL ORDER BY p.created_at DESC`,
	"credential-sets": `SELECT jsonb_build_object('id',id,'name',name,'entry_count',entry_count,'created_at',created_at) FROM credential_sets WHERE retired_at IS NULL ORDER BY created_at DESC`,
	"profiles":        `SELECT jsonb_build_object('id',id,'name',name,'config',config,'created_at',created_at,'updated_at',updated_at) FROM scan_profiles ORDER BY created_at DESC`,
	"schedules":       `SELECT jsonb_build_object('id',id,'name',name,'config',config,'cron',cron,'timezone',timezone,'enabled',enabled,'next_run_at',next_run_at,'last_run_at',last_run_at,'created_at',created_at,'misfire_policy',misfire_policy,'overlap_policy',overlap_policy,'last_occurrence',(SELECT jsonb_build_object('scheduled_at',o.scheduled_at,'state',o.state,'message',o.message,'job_id',o.job_id) FROM schedule_occurrences o WHERE o.schedule_id=schedules.id ORDER BY o.scheduled_at DESC LIMIT 1)) FROM schedules ORDER BY created_at DESC`,
	"runners":         `SELECT jsonb_build_object('id',id,'name',COALESCE(stats->>'name','runner'),'state',CASE WHEN heartbeat_at>now()-interval '30 seconds' THEN 'online' ELSE 'offline' END,'hostname',stats->>'hostname','capabilities',CASE WHEN jsonb_typeof(capabilities)='array' THEN capabilities ELSE '[]'::jsonb END,'slots',slots,'active_jobs',(SELECT count(*) FROM attempts a WHERE a.runner_id=r.id AND a.state='running'),'last_seen_at',heartbeat_at,'rss_bytes',stats->'rss_bytes','cpu_percent',stats->'cpu_percent','scanner_version',stats->>'scanner_version') FROM runners r ORDER BY heartbeat_at DESC`,
}

func (s *Store) ListResources(ctx context.Context, kind string) ([]json.RawMessage, error) {
	out := make([]json.RawMessage, 0)
	q, ok := resourceQueries[kind]
	if !ok {
		return out, ErrInvalidInput
	}
	rows, e := s.pool.Query(ctx, q)
	if e != nil {
		return out, e
	}
	defer rows.Close()
	for rows.Next() {
		var j json.RawMessage
		if e = rows.Scan(&j); e != nil {
			return out, e
		}
		out = append(out, j)
	}
	return out, rows.Err()
}

type ResourceInput struct {
	ID, VersionID, Name          string
	Config                       config.ScanConfig
	Sealed                       []byte
	Metadata                     json.RawMessage
	EntryCount                   int
	Cron, Timezone               string
	MisfirePolicy, OverlapPolicy string
	Enabled                      bool
	NextRunAt                    *time.Time
}

func (s *Store) SaveResource(ctx context.Context, kind string, in ResourceInput, update bool) (json.RawMessage, error) {
	if strings.TrimSpace(in.Name) == "" || len(in.Name) > 200 {
		return nil, &FieldError{"name", "provide 1-200 characters"}
	}
	if _, e := uuid.Parse(in.ID); e != nil {
		return nil, ErrInvalidInput
	}
	tx, e := s.pool.Begin(ctx)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback(ctx)
	cfg, e := json.Marshal(in.Config)
	if e != nil {
		return nil, ErrInvalidInput
	}
	switch kind {
	case "proxies":
		if update {
			tag, e := tx.Exec(ctx, `UPDATE proxy_profiles SET name=$2 WHERE id=$1 AND retired_at IS NULL`, in.ID, in.Name)
			if e != nil {
				return nil, e
			}
			if tag.RowsAffected() == 0 {
				return nil, ErrNotFound
			}
		} else {
			if _, e = tx.Exec(ctx, `INSERT INTO proxy_profiles(id,name) VALUES($1,$2)`, in.ID, in.Name); e != nil {
				return nil, e
			}
		}
		if _, e = tx.Exec(ctx, `INSERT INTO proxy_versions(id,profile_id,version,sealed_url,metadata) SELECT $1,$2,COALESCE(max(version),0)+1,$3,$4 FROM proxy_versions WHERE profile_id=$2`, in.VersionID, in.ID, in.Sealed, in.Metadata); e != nil {
			return nil, e
		}
	case "credential-sets":
		if update {
			return nil, ErrInvalidInput
		}
		if _, e = tx.Exec(ctx, `INSERT INTO credential_sets(id,name,sealed_credentials,entry_count) VALUES($1,$2,$3,$4)`, in.ID, in.Name, in.Sealed, in.EntryCount); e != nil {
			return nil, e
		}
	case "profiles", "schedules":
		if kind == "schedules" {
			if in.MisfirePolicy == "" {
				in.MisfirePolicy = "skip"
			}
			if in.OverlapPolicy == "" {
				in.OverlapPolicy = "skip"
			}
			if (in.MisfirePolicy != "skip" && in.MisfirePolicy != "catch_up_once") || (in.OverlapPolicy != "skip" && in.OverlapPolicy != "queue") {
				return nil, ErrInvalidInput
			}
		}
		if e = validateReferences(ctx, tx, in.Config); e != nil {
			return nil, e
		}
		var tag pgconnTag
		_ = tag
		if kind == "profiles" {
			if update {
				r, e := tx.Exec(ctx, `UPDATE scan_profiles SET name=$2,config=$3,updated_at=now() WHERE id=$1`, in.ID, in.Name, cfg)
				if e != nil {
					return nil, e
				}
				if r.RowsAffected() == 0 {
					return nil, ErrNotFound
				}
			} else {
				_, e = tx.Exec(ctx, `INSERT INTO scan_profiles(id,name,config) VALUES($1,$2,$3)`, in.ID, in.Name, cfg)
			}
		} else {
			if update {
				r, e := tx.Exec(ctx, `UPDATE schedules SET name=$2,config=$3,cron=$4,timezone=$5,enabled=$6,next_run_at=$7,misfire_policy=$8,overlap_policy=$9 WHERE id=$1`, in.ID, in.Name, cfg, in.Cron, in.Timezone, in.Enabled, in.NextRunAt, in.MisfirePolicy, in.OverlapPolicy)
				if e != nil {
					return nil, e
				}
				if r.RowsAffected() == 0 {
					return nil, ErrNotFound
				}
			} else {
				_, e = tx.Exec(ctx, `INSERT INTO schedules(id,name,config,cron,timezone,enabled,next_run_at,misfire_policy,overlap_policy) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, in.ID, in.Name, cfg, in.Cron, in.Timezone, in.Enabled, in.NextRunAt, in.MisfirePolicy, in.OverlapPolicy)
			}
		}
		if e != nil {
			return nil, e
		}
	default:
		return nil, ErrInvalidInput
	}
	if e = audit(ctx, tx, kind+".save", in.ID); e != nil {
		return nil, e
	}
	if e = tx.Commit(ctx); e != nil {
		return nil, e
	}
	return s.Resource(ctx, kind, in.ID)
}

type pgconnTag struct{}

func (s *Store) DeleteResource(ctx context.Context, kind, id string) error {
	tx, e := s.pool.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	table := map[string]string{"proxies": "proxy_profiles", "credential-sets": "credential_sets", "profiles": "scan_profiles", "schedules": "schedules"}[kind]
	if table == "" {
		return ErrInvalidInput
	}
	var locked string
	if e = tx.QueryRow(ctx, `SELECT id::text FROM `+table+` WHERE id=$1 FOR UPDATE`, id).Scan(&locked); errors.Is(e, pgx.ErrNoRows) {
		return ErrNotFound
	} else if e != nil {
		return e
	}
	if kind == "proxies" || kind == "credential-sets" {
		field := "credential_set_id"
		condition := `config->>'credential_set_id'=$1::text`
		if kind == "proxies" {
			field = "proxy_version_id"
			condition = `config->>'proxy_version_id' IN (SELECT id::text FROM proxy_versions WHERE profile_id=$1)`
		}
		_ = field
		var ref bool
		q := `SELECT EXISTS(SELECT 1 FROM jobs WHERE state IN ('queued','running','canceling') AND ` + condition + `) OR EXISTS(SELECT 1 FROM scan_profiles WHERE ` + condition + `) OR EXISTS(SELECT 1 FROM schedules WHERE ` + condition + `)`
		if e = tx.QueryRow(ctx, q, id).Scan(&ref); e != nil {
			return e
		}
		if ref {
			return ErrReferenced
		}
		_, e = tx.Exec(ctx, `UPDATE `+table+` SET retired_at=now() WHERE id=$1`, id)
	} else {
		if kind == "schedules" {
			var ref bool
			if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM schedule_occurrences WHERE schedule_id=$1)`, id).Scan(&ref); e != nil {
				return e
			}
			if ref {
				return ErrReferenced
			}
		}
		_, e = tx.Exec(ctx, `DELETE FROM `+table+` WHERE id=$1`, id)
	}
	if e != nil {
		return e
	}
	if e = audit(ctx, tx, kind+".delete", id); e != nil {
		return e
	}
	return tx.Commit(ctx)
}
func (s *Store) ProxySecret(ctx context.Context, profileID string) (string, []byte, error) {
	var id string
	var sealed []byte
	e := s.pool.QueryRow(ctx, `SELECT v.id::text,v.sealed_url FROM proxy_versions v JOIN proxy_profiles p ON p.id=v.profile_id WHERE p.id=$1 AND p.retired_at IS NULL ORDER BY version DESC LIMIT 1`, profileID).Scan(&id, &sealed)
	if errors.Is(e, pgx.ErrNoRows) {
		e = ErrNotFound
	}
	return id, sealed, e
}
func (s *Store) RecordProxyTest(ctx context.Context, id string, ok bool) error {
	tx, e := s.pool.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	message := "proxy diagnostic failed; check proxy configuration and test destination"
	if ok {
		message = "proxy diagnostic succeeded"
	}
	r, e := tx.Exec(ctx, `UPDATE proxy_profiles SET last_test_at=now(),last_test_ok=$2,last_test_message=$3 WHERE id=$1 AND retired_at IS NULL`, id, ok, message)
	if e != nil {
		return e
	}
	if r.RowsAffected() == 0 {
		return ErrNotFound
	}
	if e = audit(ctx, tx, "proxy.test", id); e != nil {
		return e
	}
	return tx.Commit(ctx)
}

type Settings struct {
	Policy            config.Policy `json:"policy"`
	RetentionDays     int           `json:"retention_days"`
	MaxConcurrentJobs int           `json:"max_concurrent_jobs"`
}

func (s *Store) GetSettings(ctx context.Context) (Settings, error) {
	var out Settings
	var raw json.RawMessage
	e := s.pool.QueryRow(ctx, `SELECT policy,retention_days,concurrency FROM settings WHERE singleton`).Scan(&raw, &out.RetentionDays, &out.MaxConcurrentJobs)
	if e != nil {
		return out, e
	}
	out.Policy = config.DefaultPolicy()
	if string(raw) != "{}" {
		if e = json.Unmarshal(raw, &out.Policy); e != nil {
			return out, e
		}
	}
	return out, nil
}
func (s *Store) PutSettings(ctx context.Context, in Settings) error {
	raw, e := json.Marshal(in.Policy)
	if e != nil {
		return ErrInvalidInput
	}
	tx, e := s.pool.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	if _, e = tx.Exec(ctx, `UPDATE settings SET policy=$1,retention_days=$2,concurrency=$3 WHERE singleton`, raw, in.RetentionDays, in.MaxConcurrentJobs); e != nil {
		return e
	}
	if e = audit(ctx, tx, "settings.update", "settings"); e != nil {
		return e
	}
	return tx.Commit(ctx)
}

// GlobalEvents shares the DB-enforced commit-safe cursor with per-job streams.
func (s *Store) GlobalEvents(ctx context.Context, id string, after int64, limit int) ([]Event, error) {
	out := make([]Event, 0)
	if after < 0 || limit < 1 || limit > 500 {
		return out, ErrInvalidInput
	}
	rows, e := s.pool.Query(ctx, `SELECT seq,job_id::text,kind,data,created_at FROM job_events WHERE ($1='' OR job_id::text=$1) AND seq>$2 ORDER BY seq LIMIT $3`, id, after, limit)
	if e != nil {
		return out, e
	}
	defer rows.Close()
	for rows.Next() {
		var v Event
		if e = rows.Scan(&v.Seq, &v.JobID, &v.Kind, &v.Data, &v.CreatedAt); e != nil {
			return out, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// Results returns stored, already sanitized worker evidence. Workers must use a
// redacting import boundary; HTTP performs a second redaction before returning.
func (s *Store) Results(ctx context.Context, jobID, state, search string, limit, offset int) ([]json.RawMessage, int64, error) {
	out := make([]json.RawMessage, 0)
	var count int64
	where := ` FROM results WHERE ($1='' OR job_id::text=$1) AND ($2='' OR state=$2) AND ($3='' OR strpos(lower(target||' '||brand),lower($3))>0)`
	if e := s.pool.QueryRow(ctx, `SELECT count(*)`+where, jobID, state, search).Scan(&count); e != nil {
		return out, 0, e
	}
	rows, e := s.pool.Query(ctx, `SELECT jsonb_build_object('id',id::text,'job_id',job_id,'target',target,'brand',brand,'state',state,'result',data)`+where+` ORDER BY id LIMIT $4 OFFSET $5`, jobID, state, search, limit, offset)
	if e != nil {
		return out, 0, e
	}
	defer rows.Close()
	for rows.Next() {
		var v json.RawMessage
		if e = rows.Scan(&v); e != nil {
			return out, 0, e
		}
		out = append(out, v)
	}
	return out, count, rows.Err()
}
func (s *Store) Overview(ctx context.Context, days int) (map[string]any, error) {
	out := map[string]any{}
	counts := map[string]int64{"queued": 0, "running": 0, "completed": 0, "inconclusive": 0, "failed": 0, "canceled": 0, "timed_out": 0, "orphaned": 0}
	rows, e := s.pool.Query(ctx, `SELECT state,count(*) FROM jobs GROUP BY state`)
	if e != nil {
		return nil, e
	}
	for rows.Next() {
		var state string
		var n int64
		if e = rows.Scan(&state, &n); e != nil {
			rows.Close()
			return nil, e
		}
		counts[state] = n
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return nil, e
	}
	out["counts"] = counts
	var result, camera, stream int64
	if e = s.pool.QueryRow(ctx, `SELECT COALESCE(sum(result_count),0),COALESCE(sum(camera_count),0),COALESCE(sum(stream_count),0) FROM jobs`).Scan(&result, &camera, &stream); e != nil {
		return nil, e
	}
	out["result_count"] = result
	out["camera_count"] = camera
	out["stream_count"] = stream
	series := make([]json.RawMessage, 0)
	rows, e = s.pool.Query(ctx, `SELECT jsonb_build_object('day',to_char(d,'YYYY-MM-DD'),'jobs',count(j.id),'completed',count(j.id) FILTER(WHERE j.state='completed'),'failed',count(j.id) FILTER(WHERE j.state='failed')) FROM generate_series((now() AT TIME ZONE 'UTC')::date-($1::int-1), (now() AT TIME ZONE 'UTC')::date,interval '1 day') d LEFT JOIN jobs j ON (j.created_at AT TIME ZONE 'UTC')::date=d::date GROUP BY d ORDER BY d`, days)
	if e != nil {
		return nil, e
	}
	for rows.Next() {
		var v json.RawMessage
		if e = rows.Scan(&v); e != nil {
			rows.Close()
			return nil, e
		}
		series = append(series, v)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return nil, e
	}
	out["series"] = series
	jobs, e := s.ListJobs(ctx, ListJobsInput{Limit: 10})
	if e != nil {
		return nil, e
	}
	out["recent_jobs"] = jobs
	runners, e := s.ListResources(ctx, "runners")
	out["runners"] = runners
	return out, e
}

type Artifact struct {
	RelativePath, MediaType  string
	JobID, AttemptID, Digest string
	Size                     int64
}

func (s *Store) Artifact(ctx context.Context, id string) (Artifact, error) {
	var a Artifact
	e := s.pool.QueryRow(ctx, `SELECT relative_path,media_type,size_bytes,job_id::text,COALESCE(attempt_id::text,''),sha256 FROM artifacts WHERE id=$1`, id).Scan(&a.RelativePath, &a.MediaType, &a.Size, &a.JobID, &a.AttemptID, &a.Digest)
	if errors.Is(e, pgx.ErrNoRows) {
		e = ErrNotFound
	}
	return a, e
}

var _ = fmt.Sprintf
