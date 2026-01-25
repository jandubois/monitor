export type ProbeStatus = 'ok' | 'warning' | 'critical' | 'unknown';

export interface Watcher {
  id: number;
  name: string;
  healthy: boolean;
  paused: boolean;
  approved: boolean;
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
  output?: OutputSchema;
  default_interval?: string;
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

export interface MetricSpec {
  type: string;
  unit?: string;
  description?: string;
}

export interface DataSpec {
  type: string;
  description?: string;
}

export interface OutputSchema {
  metrics?: Record<string, MetricSpec>;
  data?: Record<string, DataSpec>;
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
  last_summary?: string;
  last_message?: string;
  last_executed_at?: string;
}

export interface ProbeResult {
  id: number;
  probe_config_id: number;
  config_name?: string;
  watcher_id?: number;
  status: ProbeStatus;
  summary?: string;
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
  server_name: string;
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

export type EventSeverity = 'info' | 'warning' | 'error';

export interface WatcherEvent {
  id: number;
  watcher_id: number;
  watcher_name: string;
  timestamp: string;
  event_type: string;
  severity: EventSeverity;
  summary: string;
  details?: Record<string, unknown>;
  acknowledged: boolean;
  acknowledged_at?: string;
}
