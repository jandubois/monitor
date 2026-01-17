export type ProbeStatus = 'ok' | 'warning' | 'critical' | 'unknown';

export interface ProbeType {
  id: number;
  name: string;
  description: string;
  version: string;
  arguments: {
    required?: Record<string, ArgumentSpec>;
    optional?: Record<string, ArgumentSpec>;
  };
  executable_path: string;
  registered_at: string;
  updated_at: string | null;
}

export interface ArgumentSpec {
  type: string;
  description: string;
  default?: unknown;
}

export interface ProbeConfig {
  id: number;
  probe_type_id: number;
  probe_type_name: string;
  name: string;
  enabled: boolean;
  arguments: Record<string, unknown>;
  interval: string;
  timeout_seconds: number;
  notification_channels: number[];
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
  status: ProbeStatus;
  message: string;
  metrics: Record<string, unknown> | null;
  data: Record<string, unknown> | null;
  duration_ms: number;
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

export interface SystemStatus {
  watcher_healthy: boolean;
  watcher_last_seen: string | null;
  watcher_version?: string;
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
