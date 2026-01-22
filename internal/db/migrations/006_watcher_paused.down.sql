-- Revert: Remove paused column from watchers

ALTER TABLE watchers DROP COLUMN paused;
