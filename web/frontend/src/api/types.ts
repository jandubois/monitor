export type ProbeStatus = 'ok' | 'warning' | 'critical' | 'unknown';

export interface Watcher {
  id: number;
  name: string;
  healthy: boolean;
  paused: boolean;
  last_seen_at?: string;
  version?: string;
  registered_at: string;
  probe_type_count: number;
  config_count: number;
}

export interface WatcherDetail extends Watcher {
  probe_types: ProbeType[];
}

export interface ProbeType {
  id: number;
  name: string;
  description: string;
  version: string;
  arguments: {
    required?: Record<string, ArgumentSpec>;
    optional?: Record<string, ArgumentSpec>;
  };
  executable_path?: string;
  registered_at: string;
  updated_at: string | null;
}

export interface ArgumentSpec {
  type: string;
  description: string;
  default?: unknown;
  enum?: string[];
}

export interface ProbeConfig {
  id: number;
  probe_type_id: number;
  probe_type_name: string;
  watcher_id?: number;
  watcher_name?: string;
  name: string;
  enabled: boolean;
  arguments: Record<string, unknown>;
  interval: string;
  timeout_seconds: number;
  notification_channels: number[];
  next_run_at?: string;
  group_path?: string;
  keywords?: string[];
  created_at: string;
  updated_at: string | null;
  last_status?: ProbeStatus;
  last_message?: string;
  last_executed_at?: string;
}

export interface ProbeResult {
  id: number;
  probe_config_id: number;
  config_name?: string;
  watcher_id?: number;
  status: ProbeStatus;
  message: string;
  metrics: Record<string, unknown> | null;
  data: Record<string, unknown> | null;
  duration_ms: number;
  next_run_at?: string;
  scheduled_at: string;
  executed_at: string;
  recorded_at: string;
}

export interface NotificationChannel {
  id: number;
  name: string;
  type: string;
  config: Record<string, unknown>;
  enabled: boolean;
}

export interface WatcherStatus {
  name: string;
  healthy: boolean;
  last_seen?: string;
  version?: string;
}

export interface SystemStatus {
  watchers: WatcherStatus[];
  all_healthy: boolean;
  recent_failures: number;
}

export interface ResultStats {
  total_configs: number;
  enabled_configs: number;
  status_counts: {
    ok: number;
    warning: number;
    critical: number;
    unknown: number;
  };
}

export interface ProbeConfigFilters {
  watcher?: number;
  group?: string;
  keywords?: string;
}
