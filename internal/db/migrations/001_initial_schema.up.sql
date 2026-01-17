-- Watcher heartbeat for health monitoring
CREATE TABLE watcher_heartbeat (
    id INTEGER PRIMARY KEY DEFAULT 1,
    last_seen_at TIMESTAMPTZ,
    watcher_version TEXT,
    CONSTRAINT single_row CHECK (id = 1)
);

-- Insert initial row
INSERT INTO watcher_heartbeat (id) VALUES (1);

-- Registered probe types (discovered via --describe)
CREATE TABLE probe_types (
    id SERIAL PRIMARY KEY,
    name TEXT UNIQUE NOT NULL,
    description TEXT,
    version TEXT,
    arguments JSONB,
    executable_path TEXT NOT NULL,
    registered_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ
);

-- Notification channels
CREATE TABLE notification_channels (
    id SERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    type TEXT NOT NULL,
    config JSONB,
    enabled BOOLEAN DEFAULT true
);

-- Configured probe instances
CREATE TABLE probe_configs (
    id SERIAL PRIMARY KEY,
    probe_type_id INTEGER NOT NULL REFERENCES probe_types(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    enabled BOOLEAN DEFAULT true,
    arguments JSONB,
    interval TEXT NOT NULL,
    timeout_seconds INTEGER DEFAULT 60,
    notification_channels INTEGER[],
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ
);

-- Probe execution results
CREATE TABLE probe_results (
    id SERIAL PRIMARY KEY,
    probe_config_id INTEGER NOT NULL REFERENCES probe_configs(id) ON DELETE CASCADE,
    status TEXT NOT NULL,
    message TEXT,
    metrics JSONB,
    data JSONB,
    duration_ms INTEGER,
    scheduled_at TIMESTAMPTZ,
    executed_at TIMESTAMPTZ,
    recorded_at TIMESTAMPTZ DEFAULT NOW()
);

-- Missed runs tracking
CREATE TABLE missed_runs (
    id SERIAL PRIMARY KEY,
    probe_config_id INTEGER NOT NULL REFERENCES probe_configs(id) ON DELETE CASCADE,
    scheduled_at TIMESTAMPTZ,
    reason TEXT
);

-- Indexes for efficient queries
CREATE INDEX idx_results_config_time ON probe_results(probe_config_id, executed_at DESC);
CREATE INDEX idx_results_status ON probe_results(status) WHERE status != 'ok';
CREATE INDEX idx_results_executed ON probe_results(executed_at DESC);
CREATE INDEX idx_missed_runs_config ON missed_runs(probe_config_id);
