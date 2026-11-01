-- Application lifecycle states are independent of River's job_state enum.
-- In particular, River continues to use 'cancelled'.
ALTER TABLE jobs DROP CONSTRAINT jobs_state_check;
ALTER TABLE attempts DROP CONSTRAINT attempts_state_check;

UPDATE jobs SET state = CASE state
 WHEN 'succeeded' THEN 'completed'
 WHEN 'cancelled' THEN 'canceled'
 WHEN 'interrupted' THEN 'orphaned'
 ELSE state END
WHERE state IN ('succeeded','cancelled','interrupted');
UPDATE attempts SET state = CASE state
 WHEN 'succeeded' THEN 'completed'
 WHEN 'cancelled' THEN 'canceled'
 WHEN 'interrupted' THEN 'orphaned'
 ELSE state END
WHERE state IN ('succeeded','cancelled','interrupted');

ALTER TABLE jobs ADD CONSTRAINT jobs_state_check CHECK (
 state IN ('queued','running','canceling','completed','inconclusive','failed','canceled','timed_out','orphaned')
);
ALTER TABLE attempts ADD CONSTRAINT attempts_state_check CHECK (
 state IN ('queued','running','canceling','completed','inconclusive','failed','canceled','timed_out','orphaned')
);
