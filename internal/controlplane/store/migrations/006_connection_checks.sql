CREATE TABLE connection_checks (
 id uuid PRIMARY KEY,
 result_id bigint NOT NULL REFERENCES results(id) ON DELETE CASCADE,
 profile_id text NOT NULL DEFAULT '',
 outcome jsonb NOT NULL CHECK (octet_length(outcome::text) <= 1048576),
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX connection_checks_result_created_idx ON connection_checks(result_id,created_at DESC,id);
