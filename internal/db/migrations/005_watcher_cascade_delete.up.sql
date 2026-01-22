-- Migration: Watcher cascade delete and cleanup
-- Enables deleting watchers with all associated data

-- 1. Make probe_configs.watcher_id cascade on delete
ALTER TABLE probe_configs DROP CONSTRAINT IF EXISTS probe_configs_watcher_id_fkey;
ALTER TABLE probe_configs ADD CONSTRAINT probe_configs_watcher_id_fkey
    FOREIGN KEY (watcher_id) REFERENCES watchers(id) ON DELETE CASCADE;

-- 2. Make probe_results.watcher_id cascade on delete
ALTER TABLE probe_results DROP CONSTRAINT IF EXISTS probe_results_watcher_id_fkey;
ALTER TABLE probe_results ADD CONSTRAINT probe_results_watcher_id_fkey
    FOREIGN KEY (watcher_id) REFERENCES watchers(id) ON DELETE CASCADE;

-- 3. Remove auto-created 'default' watcher if it has no probe configs
DELETE FROM watchers
WHERE name = 'default'
  AND NOT EXISTS (SELECT 1 FROM probe_configs WHERE watcher_id = watchers.id);
