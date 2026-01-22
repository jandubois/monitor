-- Add token and approval status to watchers
ALTER TABLE watchers ADD COLUMN token TEXT;
ALTER TABLE watchers ADD COLUMN approved INTEGER NOT NULL DEFAULT 0;

-- Unique index on token (partial index for non-null tokens)
CREATE UNIQUE INDEX idx_watchers_token ON watchers(token) WHERE token IS NOT NULL;
