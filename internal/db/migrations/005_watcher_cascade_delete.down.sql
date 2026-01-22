-- Revert: Watcher cascade delete
-- Removes CASCADE from foreign keys (cannot restore deleted 'default' watcher)

-- 1. Remove cascade from probe_configs.watcher_id
ALTER TABLE probe_configs DROP CONSTRAINT IF EXISTS probe_configs_watcher_id_fkey;
ALTER TABLE probe_configs ADD CONSTRAINT probe_configs_watcher_id_fkey
    FOREIGN KEY (watcher_id) REFERENCES watchers(id);

-- 2. Remove cascade from probe_results.watcher_id
ALTER TABLE probe_results DROP CONSTRAINT IF EXISTS probe_results_watcher_id_fkey;
ALTER TABLE probe_results ADD CONSTRAINT probe_results_watcher_id_fkey
    FOREIGN KEY (watcher_id) REFERENCES watchers(id);
