-- Rebuild watchers table with AUTOINCREMENT to prevent ID reuse.
-- SQLite requires a table rebuild to add AUTOINCREMENT.
-- Foreign keys must be disabled during the rebuild to prevent
-- ON DELETE CASCADE from deleting related rows when we drop the old table.

PRAGMA foreign_keys = OFF;

CREATE TABLE watchers_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT UNIQUE NOT NULL,
    last_seen_at TEXT,
    version TEXT,
    callback_url TEXT,
    paused INTEGER NOT NULL DEFAULT 0,
    registered_at TEXT DEFAULT (datetime('now')),
    token TEXT,
    approved INTEGER NOT NULL DEFAULT 0
);

INSERT INTO watchers_new SELECT * FROM watchers;

DROP TABLE watchers;

ALTER TABLE watchers_new RENAME TO watchers;

-- Recreate the token index from migration 002
CREATE UNIQUE INDEX idx_watchers_token ON watchers(token) WHERE token IS NOT NULL;

PRAGMA foreign_keys = ON;
