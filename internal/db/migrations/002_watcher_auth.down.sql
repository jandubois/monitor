-- Remove token authentication columns from watchers
DROP INDEX IF EXISTS idx_watchers_token;

-- SQLite doesn't support DROP COLUMN directly, so we need to recreate the table
CREATE TABLE watchers_new (
    id INTEGER PRIMARY KEY,
    name TEXT UNIQUE NOT NULL,
    last_seen_at TEXT,
    version TEXT,
    callback_url TEXT,
    paused INTEGER NOT NULL DEFAULT 0,
    registered_at TEXT DEFAULT (datetime('now'))
);

INSERT INTO watchers_new (id, name, last_seen_at, version, callback_url, paused, registered_at)
SELECT id, name, last_seen_at, version, callback_url, paused, registered_at FROM watchers;

DROP TABLE watchers;
ALTER TABLE watchers_new RENAME TO watchers;
