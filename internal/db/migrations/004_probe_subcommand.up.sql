-- Add subcommand support for multi-probe binaries
ALTER TABLE watcher_probe_types ADD COLUMN subcommand TEXT;
