CREATE TABLE jobs (
 id uuid PRIMARY KEY,
 name text NOT NULL CHECK (length(name) BETWEEN 1 AND 200),
 state text NOT NULL DEFAULT 'queued' CHECK (state IN ('queued','running','succeeded','failed','cancelled','interrupted')),
 phase text NOT NULL DEFAULT 'queued', config jsonb NOT NULL,
 priority smallint NOT NULL CHECK(priority BETWEEN 1 AND 4),
 created_at timestamptz NOT NULL DEFAULT now(), started_at timestamptz, finished_at timestamptz,
 error text NOT NULL DEFAULT '', result_count bigint NOT NULL DEFAULT 0,
 camera_count bigint NOT NULL DEFAULT 0, stream_count bigint NOT NULL DEFAULT 0,
 inconclusive_count bigint NOT NULL DEFAULT 0, cancel_requested boolean NOT NULL DEFAULT false,
 attempt integer NOT NULL DEFAULT 0, retry_of uuid REFERENCES jobs(id),
 idempotency_key text UNIQUE, request_hash bytea NOT NULL,
 river_job_id bigint UNIQUE REFERENCES river_job(id) ON DELETE SET NULL
);
CREATE INDEX jobs_created_idx ON jobs(created_at DESC,id DESC);
CREATE INDEX jobs_state_created_idx ON jobs(state,created_at DESC);
CREATE FUNCTION immutable_job_config() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.config IS DISTINCT FROM OLD.config OR NEW.request_hash IS DISTINCT FROM OLD.request_hash OR NEW.idempotency_key IS DISTINCT FROM OLD.idempotency_key THEN
  RAISE EXCEPTION 'job request is immutable';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER immutable_job_config BEFORE UPDATE ON jobs FOR EACH ROW EXECUTE FUNCTION immutable_job_config();
CREATE TABLE runners (
 id uuid PRIMARY KEY, heartbeat_at timestamptz NOT NULL DEFAULT now(),
 capabilities jsonb NOT NULL DEFAULT '{}', slots integer NOT NULL CHECK(slots>=0), stats jsonb NOT NULL DEFAULT '{}'
);
CREATE TABLE attempts (
 id uuid PRIMARY KEY, job_id uuid NOT NULL REFERENCES jobs(id), number integer NOT NULL CHECK(number>0),
 fencing_token uuid NOT NULL UNIQUE, state text NOT NULL CHECK(state IN ('running','succeeded','failed','cancelled','interrupted')),
 runner_id uuid REFERENCES runners(id), started_at timestamptz NOT NULL DEFAULT now(), finished_at timestamptz,
 UNIQUE(job_id,number), UNIQUE(job_id,id)
);
CREATE TABLE job_events (
 seq bigserial PRIMARY KEY, job_id uuid NOT NULL REFERENCES jobs(id),
 kind text NOT NULL CHECK(length(kind) BETWEEN 1 AND 100), data jsonb NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX job_events_job_seq_idx ON job_events(job_id,seq);
CREATE TABLE results (
 id bigserial PRIMARY KEY, job_id uuid NOT NULL, attempt_id uuid NOT NULL,
 target text NOT NULL, brand text NOT NULL, state text NOT NULL, data jsonb NOT NULL,
 FOREIGN KEY(job_id,attempt_id) REFERENCES attempts(job_id,id), UNIQUE(job_id,attempt_id,target)
);
CREATE INDEX results_job_idx ON results(job_id,id);
CREATE INDEX results_filter_idx ON results(job_id,brand,state);
CREATE TABLE proxy_profiles (id uuid PRIMARY KEY, name text NOT NULL, created_at timestamptz NOT NULL DEFAULT now());
CREATE TABLE proxy_versions (
 id uuid PRIMARY KEY, profile_id uuid NOT NULL REFERENCES proxy_profiles(id),
 version integer NOT NULL CHECK(version>0), sealed_url bytea NOT NULL CHECK(octet_length(sealed_url)>=29),
 created_at timestamptz NOT NULL DEFAULT now(), UNIQUE(profile_id,version)
);
CREATE FUNCTION immutable_proxy_version() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN RAISE EXCEPTION 'proxy versions are immutable'; END $$;
CREATE TRIGGER immutable_proxy_version BEFORE UPDATE ON proxy_versions FOR EACH ROW EXECUTE FUNCTION immutable_proxy_version();
CREATE TABLE credential_sets (
 id uuid PRIMARY KEY, name text NOT NULL, sealed_credentials bytea NOT NULL CHECK(octet_length(sealed_credentials)>=29),
 entry_count integer NOT NULL CHECK(entry_count>=0), created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE scan_profiles (id uuid PRIMARY KEY, name text NOT NULL, config jsonb NOT NULL, created_at timestamptz NOT NULL DEFAULT now());
CREATE TABLE schedules (
 id uuid PRIMARY KEY, name text NOT NULL, config jsonb NOT NULL, cron text NOT NULL,
 timezone text NOT NULL, enabled boolean NOT NULL DEFAULT false, next_run_at timestamptz,
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE schedule_occurrences (
 schedule_id uuid NOT NULL REFERENCES schedules(id), scheduled_at timestamptz NOT NULL,
 job_id uuid UNIQUE REFERENCES jobs(id), PRIMARY KEY(schedule_id,scheduled_at)
);
CREATE TABLE artifacts (
 id uuid PRIMARY KEY, job_id uuid NOT NULL REFERENCES jobs(id), attempt_id uuid,
 relative_path text NOT NULL CHECK(relative_path<>'' AND relative_path !~ '(^/|(^|/)\.\.(/|$)|\\)' ),
 media_type text NOT NULL, size_bytes bigint NOT NULL CHECK(size_bytes>=0),
 FOREIGN KEY(job_id,attempt_id) REFERENCES attempts(job_id,id), UNIQUE(job_id,relative_path)
);
CREATE TABLE settings (
 singleton boolean PRIMARY KEY DEFAULT true CHECK(singleton), policy jsonb NOT NULL DEFAULT '{}',
 retention_days integer NOT NULL DEFAULT 30 CHECK(retention_days>0), concurrency integer NOT NULL DEFAULT 1 CHECK(concurrency>0)
);
INSERT INTO settings(singleton) VALUES(true);
CREATE TABLE audit_events (
 seq bigserial PRIMARY KEY, created_at timestamptz NOT NULL DEFAULT now(),
 actor text NOT NULL, action text NOT NULL, resource_id text NOT NULL, metadata jsonb NOT NULL DEFAULT '{}'
);
