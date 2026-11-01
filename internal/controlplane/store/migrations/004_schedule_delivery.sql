ALTER TABLE schedules ADD COLUMN misfire_policy text NOT NULL DEFAULT 'skip' CHECK(misfire_policy IN ('skip','catch_up_once'));
ALTER TABLE schedules ADD COLUMN overlap_policy text NOT NULL DEFAULT 'skip' CHECK(overlap_policy IN ('skip','queue'));
ALTER TABLE schedule_occurrences ADD COLUMN state text NOT NULL DEFAULT 'queued';
ALTER TABLE schedule_occurrences ADD COLUMN message text NOT NULL DEFAULT '';
CREATE INDEX schedules_due_idx ON schedules(next_run_at) WHERE enabled;
