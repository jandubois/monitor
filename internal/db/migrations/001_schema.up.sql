-- SQLite schema for Monitor

-- Registered watchers
CREATE TABLE watchers (
    id INTEGER PRIMARY KEY,
    name TEXT UNIQUE NOT NULL,
    last_seen_at TEXT,
    version TEXT,
    callback_url TEXT,
    paused INTEGER NOT NULL DEFAULT 0,
    registered_at TEXT DEFAULT (datetime('now'))
);

-- Registered probe types (discovered via --describe)
CREATE TABLE probe_types (
    id INTEGER PRIMARY KEY,
    name TEXT NOT NULL,
    version TEXT NOT NULL,
    description TEXT,
    arguments TEXT,  -- JSON
    registered_at TEXT DEFAULT (datetime('now')),
    updated_at TEXT,
    UNIQUE(name, version)
);

-- Which probe types are available on which watchers
CREATE TABLE watcher_probe_types (
    watcher_id INTEGER NOT NULL REFERENCES watchers(id) ON DELETE CASCADE,
    probe_type_id INTEGER NOT NULL REFERENCES probe_types(id) ON DELETE CASCADE,
    executable_path TEXT NOT NULL,
    subcommand TEXT,
    PRIMARY KEY (watcher_id, probe_type_id)
);

-- Notification channels
CREATE TABLE notification_channels (
    id INTEGER PRIMARY KEY,
    name TEXT NOT NULL,
    type TEXT NOT NULL,
    config TEXT,  -- JSON
    enabled INTEGER DEFAULT 1
);

-- Configured probe instances
CREATE TABLE probe_configs (
    id INTEGER PRIMARY KEY,
    probe_type_id INTEGER NOT NULL REFERENCES probe_types(id) ON DELETE CASCADE,
    watcher_id INTEGER REFERENCES watchers(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    enabled INTEGER DEFAULT 1,
    arguments TEXT,  -- JSON
    interval TEXT NOT NULL,
    timeout_seconds INTEGER DEFAULT 60,
    notification_channels TEXT,  -- JSON array of IDs
    next_run_at TEXT,
    group_path TEXT,
    keywords TEXT,  -- JSON array of strings
    created_at TEXT DEFAULT (datetime('now')),
    updated_at TEXT
);

-- Probe execution results
CREATE TABLE probe_results (
    id INTEGER PRIMARY KEY,
    probe_config_id INTEGER NOT NULL REFERENCES probe_configs(id) ON DELETE CASCADE,
    watcher_id INTEGER REFERENCES watchers(id) ON DELETE CASCADE,
    status TEXT NOT NULL,
    message TEXT,
    metrics TEXT,  -- JSON
    data TEXT,  -- JSON
    duration_ms INTEGER,
    next_run_at TEXT,
    scheduled_at TEXT,
    executed_at TEXT,
    recorded_at TEXT DEFAULT (datetime('now'))
);

-- Missed runs tracking
CREATE TABLE missed_runs (
    id INTEGER PRIMARY KEY,
    probe_config_id INTEGER NOT NULL REFERENCES probe_configs(id) ON DELETE CASCADE,
    scheduled_at TEXT,
    reason TEXT
);

-- Indexes
CREATE INDEX idx_results_config_time ON probe_results(probe_config_id, executed_at DESC);
CREATE INDEX idx_results_status ON probe_results(status) WHERE status != 'ok';
CREATE INDEX idx_results_executed ON probe_results(executed_at DESC);
CREATE INDEX idx_configs_watcher ON probe_configs(watcher_id) WHERE enabled = 1;
CREATE INDEX idx_configs_group ON probe_configs(group_path) WHERE group_path IS NOT NULL;
CREATE INDEX idx_missed_runs_config ON missed_runs(probe_config_id);
