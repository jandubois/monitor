PRAGMA foreign_keys = OFF;

CREATE TABLE watchers_new (
    id INTEGER PRIMARY KEY,
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

CREATE UNIQUE INDEX idx_watchers_token ON watchers(token) WHERE token IS NOT NULL;

PRAGMA foreign_keys = ON;
