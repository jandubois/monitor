-- Rollback: Multi-watcher support
-- Reverts to single-watcher architecture

-- 1. Add executable_path back to probe_types
ALTER TABLE probe_types ADD COLUMN executable_path TEXT;

-- 2. Restore executable_path from watcher_probe_types (use first match)
UPDATE probe_types pt
SET executable_path = (
    SELECT wpt.executable_path
    FROM watcher_probe_types wpt
    WHERE wpt.probe_type_id = pt.id
    LIMIT 1
);

-- 3. Recreate watcher_heartbeat table
CREATE TABLE watcher_heartbeat (
    id INTEGER PRIMARY KEY DEFAULT 1,
    last_seen_at TIMESTAMPTZ,
    watcher_version TEXT,
    CONSTRAINT single_row CHECK (id = 1)
);

-- 4. Migrate default watcher data back to watcher_heartbeat
INSERT INTO watcher_heartbeat (id, last_seen_at, watcher_version)
SELECT 1, last_seen_at, version
FROM watchers
WHERE name = 'default'
LIMIT 1;

-- Insert empty row if no default watcher existed
INSERT INTO watcher_heartbeat (id)
SELECT 1
WHERE NOT EXISTS (SELECT 1 FROM watcher_heartbeat);

-- 5. Drop new indexes
DROP INDEX IF EXISTS idx_configs_keywords;
DROP INDEX IF EXISTS idx_configs_group;
DROP INDEX IF EXISTS idx_configs_watcher;

-- 6. Remove new columns from probe_results
ALTER TABLE probe_results DROP COLUMN IF EXISTS next_run_at;
ALTER TABLE probe_results DROP COLUMN IF EXISTS watcher_id;

-- 7. Remove new columns from probe_configs
ALTER TABLE probe_configs DROP COLUMN IF EXISTS keywords;
ALTER TABLE probe_configs DROP COLUMN IF EXISTS group_path;
ALTER TABLE probe_configs DROP COLUMN IF EXISTS next_run_at;
ALTER TABLE probe_configs DROP COLUMN IF EXISTS watcher_id;

-- 8. Drop watcher_probe_types junction table
DROP TABLE watcher_probe_types;

-- 9. Drop watchers table
DROP TABLE watchers;

-- 10. Revert probe_types unique constraint
ALTER TABLE probe_types DROP CONSTRAINT IF EXISTS probe_types_name_version_key;
ALTER TABLE probe_types ALTER COLUMN version DROP NOT NULL;
ALTER TABLE probe_types ADD CONSTRAINT probe_types_name_key UNIQUE (name);
