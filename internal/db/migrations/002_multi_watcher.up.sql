-- Migration: Multi-watcher support
-- Transforms single-watcher architecture to support multiple watchers
-- communicating via HTTP push API

-- 1. Create watchers table (replaces watcher_heartbeat)
CREATE TABLE watchers (
    id SERIAL PRIMARY KEY,
    name TEXT UNIQUE NOT NULL,
    last_seen_at TIMESTAMPTZ,
    version TEXT,
    registered_at TIMESTAMPTZ DEFAULT NOW()
);

-- 2. Create watcher_probe_types junction table
-- Tracks which probe types are available on which watchers
CREATE TABLE watcher_probe_types (
    watcher_id INTEGER NOT NULL REFERENCES watchers(id) ON DELETE CASCADE,
    probe_type_id INTEGER NOT NULL REFERENCES probe_types(id) ON DELETE CASCADE,
    executable_path TEXT NOT NULL,
    PRIMARY KEY (watcher_id, probe_type_id)
);

-- 3. Migrate probe_types: move from UNIQUE(name) to UNIQUE(name, version)
-- First, drop the unique constraint on name
ALTER TABLE probe_types DROP CONSTRAINT probe_types_name_key;

-- Set version to '0.0.0' for any existing rows without version
UPDATE probe_types SET version = '0.0.0' WHERE version IS NULL OR version = '';

-- Make version NOT NULL
ALTER TABLE probe_types ALTER COLUMN version SET NOT NULL;

-- Add the new unique constraint on (name, version)
ALTER TABLE probe_types ADD CONSTRAINT probe_types_name_version_key UNIQUE (name, version);

-- 4. Add watcher_id and new columns to probe_configs
ALTER TABLE probe_configs ADD COLUMN watcher_id INTEGER REFERENCES watchers(id);
ALTER TABLE probe_configs ADD COLUMN next_run_at TIMESTAMPTZ;
ALTER TABLE probe_configs ADD COLUMN group_path TEXT;
ALTER TABLE probe_configs ADD COLUMN keywords TEXT[];

-- 5. Add watcher_id and next_run_at to probe_results
ALTER TABLE probe_results ADD COLUMN watcher_id INTEGER REFERENCES watchers(id);
ALTER TABLE probe_results ADD COLUMN next_run_at TIMESTAMPTZ;

-- 6. Create new indexes
CREATE INDEX idx_configs_watcher ON probe_configs(watcher_id) WHERE enabled;
CREATE INDEX idx_configs_group ON probe_configs(group_path) WHERE group_path IS NOT NULL;
CREATE INDEX idx_configs_keywords ON probe_configs USING GIN(keywords) WHERE keywords IS NOT NULL;

-- 7. Migrate existing data: create a default watcher from watcher_heartbeat
INSERT INTO watchers (name, last_seen_at, version, registered_at)
SELECT 'default', last_seen_at, watcher_version, NOW()
FROM watcher_heartbeat
WHERE id = 1 AND last_seen_at IS NOT NULL
ON CONFLICT DO NOTHING;

-- If no heartbeat existed, create a placeholder watcher anyway
INSERT INTO watchers (name, registered_at)
SELECT 'default', NOW()
WHERE NOT EXISTS (SELECT 1 FROM watchers WHERE name = 'default');

-- 8. Migrate probe_types executable_path to watcher_probe_types
-- Associate all existing probe types with the default watcher
INSERT INTO watcher_probe_types (watcher_id, probe_type_id, executable_path)
SELECT w.id, pt.id, pt.executable_path
FROM probe_types pt
CROSS JOIN watchers w
WHERE w.name = 'default' AND pt.executable_path IS NOT NULL;

-- 9. Associate existing probe_configs with the default watcher
UPDATE probe_configs
SET watcher_id = (SELECT id FROM watchers WHERE name = 'default')
WHERE watcher_id IS NULL;

-- 10. Drop watcher_heartbeat table (no longer needed)
DROP TABLE watcher_heartbeat;

-- 11. Remove executable_path from probe_types (now in watcher_probe_types)
ALTER TABLE probe_types DROP COLUMN executable_path;
