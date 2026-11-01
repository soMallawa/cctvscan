-- A statement trigger runs BEFORE evaluation of bigserial defaults. Its lock is
-- held until commit, across EVERY insert path (including future worker SQL).
-- Thus a visible seq is a safe global replay watermark; rollback leaves gaps,
-- never a lower sequence that can become visible after a higher one.
CREATE FUNCTION serialize_event_publication() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN PERFORM pg_advisory_xact_lock(739250033); RETURN NULL; END $$;
CREATE TRIGGER serialize_event_publication BEFORE INSERT ON job_events
FOR EACH STATEMENT EXECUTE FUNCTION serialize_event_publication();
ALTER TABLE proxy_profiles ADD COLUMN retired_at timestamptz;
ALTER TABLE proxy_versions ADD COLUMN metadata jsonb NOT NULL DEFAULT '{}';
ALTER TABLE proxy_profiles ADD COLUMN last_test_at timestamptz, ADD COLUMN last_test_ok boolean, ADD COLUMN last_test_message text;
ALTER TABLE credential_sets ADD COLUMN retired_at timestamptz;
ALTER TABLE scan_profiles ADD COLUMN updated_at timestamptz NOT NULL DEFAULT now();
ALTER TABLE schedules ADD COLUMN last_run_at timestamptz;
CREATE TABLE job_resource_refs (
 job_id uuid PRIMARY KEY REFERENCES jobs(id) ON DELETE CASCADE,
 proxy_version_id uuid REFERENCES proxy_versions(id), credential_set_id uuid REFERENCES credential_sets(id)
);
