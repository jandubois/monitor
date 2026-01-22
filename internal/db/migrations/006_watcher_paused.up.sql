-- Migration: Add paused column to watchers
-- Allows pausing all notifications from a watcher without modifying individual probe configs

ALTER TABLE watchers ADD COLUMN paused BOOLEAN NOT NULL DEFAULT false;
