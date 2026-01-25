-- Watcher events table for tracking connection state changes, version changes, etc.
CREATE TABLE watcher_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    watcher_id INTEGER NOT NULL REFERENCES watchers(id) ON DELETE CASCADE,
    timestamp TEXT NOT NULL DEFAULT (datetime('now')),
    event_type TEXT NOT NULL,
    severity TEXT NOT NULL DEFAULT 'info',
    summary TEXT NOT NULL,
    details TEXT,
    acknowledged INTEGER NOT NULL DEFAULT 0,
    acknowledged_at TEXT
);

CREATE INDEX idx_watcher_events_watcher ON watcher_events(watcher_id);
CREATE INDEX idx_watcher_events_timestamp ON watcher_events(timestamp);
