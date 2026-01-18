import type {
  ProbeType,
  ProbeConfig,
  ProbeResult,
  NotificationChannel,
  SystemStatus,
  ResultStats,
  Watcher,
  WatcherDetail,
  ProbeConfigFilters,
} from './types';

class ApiClient {
  private token: string | null = null;

  setToken(token: string) {
    this.token = token;
    localStorage.setItem('auth_token', token);
  }

  getToken(): string | null {
    if (!this.token) {
      this.token = localStorage.getItem('auth_token');
    }
    return this.token;
  }

  clearToken() {
    this.token = null;
    localStorage.removeItem('auth_token');
  }

  private async request<T>(path: string, options: RequestInit = {}): Promise<T> {
    const token = this.getToken();
    if (!token) {
      throw new Error('Not authenticated');
    }

    const response = await fetch(`/api${path}`, {
      ...options,
      headers: {
        'Authorization': `Bearer ${token}`,
        'Content-Type': 'application/json',
        ...options.headers,
      },
    });

    if (!response.ok) {
      if (response.status === 401) {
        this.clearToken();
        throw new Error('Unauthorized');
      }
      throw new Error(`API error: ${response.status}`);
    }

    if (response.status === 204) {
      return undefined as T;
    }

    return response.json();
  }

  async getStatus(): Promise<SystemStatus> {
    return this.request('/status');
  }

  async getResultStats(): Promise<ResultStats> {
    return this.request('/results/stats');
  }

  // Watchers
  async getWatchers(): Promise<Watcher[]> {
    return this.request('/watchers');
  }

  async getWatcher(id: number): Promise<WatcherDetail> {
    return this.request(`/watchers/${id}`);
  }

  // Probe Types
  async getProbeTypes(watcherId?: number): Promise<ProbeType[]> {
    const query = watcherId ? `?watcher=${watcherId}` : '';
    return this.request(`/probe-types${query}`);
  }

  async discoverProbeTypes(): Promise<{ message: string; probe_types: number }> {
    return this.request('/probe-types/discover', { method: 'POST' });
  }

  // Probe Configs
  async getProbeConfigs(filters?: ProbeConfigFilters): Promise<ProbeConfig[]> {
    const params = new URLSearchParams();
    if (filters?.watcher) params.set('watcher', String(filters.watcher));
    if (filters?.group) params.set('group', filters.group);
    if (filters?.keywords) params.set('keywords', filters.keywords);
    const query = params.toString();
    return this.request(`/probe-configs${query ? `?${query}` : ''}`);
  }

  async getProbeConfig(id: number): Promise<ProbeConfig> {
    return this.request(`/probe-configs/${id}`);
  }

  async createProbeConfig(config: {
    probe_type_id: number;
    watcher_id?: number;
    name: string;
    enabled: boolean;
    arguments: Record<string, unknown>;
    interval: string;
    timeout_seconds: number;
    notification_channels: number[];
    group_path?: string;
    keywords?: string[];
  }): Promise<{ id: number }> {
    return this.request('/probe-configs', {
      method: 'POST',
      body: JSON.stringify(config),
    });
  }

  async updateProbeConfig(id: number, config: {
    watcher_id?: number;
    name: string;
    enabled: boolean;
    arguments: Record<string, unknown>;
    interval: string;
    timeout_seconds: number;
    notification_channels: number[];
    group_path?: string;
    keywords?: string[];
  }): Promise<void> {
    return this.request(`/probe-configs/${id}`, {
      method: 'PUT',
      body: JSON.stringify(config),
    });
  }

  async deleteProbeConfig(id: number): Promise<void> {
    return this.request(`/probe-configs/${id}`, {
      method: 'DELETE',
    });
  }

  async triggerProbe(id: number): Promise<void> {
    return this.request(`/probe-configs/${id}/run`, {
      method: 'POST',
    });
  }

  // Results
  async getResults(params?: {
    config_id?: number;
    status?: string | string[];
    since?: string;
    limit?: number;
    offset?: number;
  }): Promise<ProbeResult[]> {
    const searchParams = new URLSearchParams();
    if (params?.config_id) searchParams.set('config_id', String(params.config_id));
    if (params?.status) {
      const statuses = Array.isArray(params.status) ? params.status.join(',') : params.status;
      searchParams.set('status', statuses);
    }
    if (params?.since) searchParams.set('since', params.since);
    if (params?.limit) searchParams.set('limit', String(params.limit));
    if (params?.offset) searchParams.set('offset', String(params.offset));
    const query = searchParams.toString();
    return this.request(`/results${query ? `?${query}` : ''}`);
  }

  async getProbeResults(configId: number): Promise<ProbeResult[]> {
    return this.request(`/results/${configId}`);
  }

  // Notification Channels
  async getNotificationChannels(): Promise<NotificationChannel[]> {
    return this.request('/notification-channels');
  }

  async createNotificationChannel(channel: Omit<NotificationChannel, 'id'>): Promise<{ id: number }> {
    return this.request('/notification-channels', {
      method: 'POST',
      body: JSON.stringify(channel),
    });
  }

  async updateNotificationChannel(id: number, channel: Partial<NotificationChannel>): Promise<void> {
    return this.request(`/notification-channels/${id}`, {
      method: 'PUT',
      body: JSON.stringify(channel),
    });
  }

  async deleteNotificationChannel(id: number): Promise<void> {
    return this.request(`/notification-channels/${id}`, {
      method: 'DELETE',
    });
  }

  async testNotificationChannel(id: number): Promise<void> {
    return this.request(`/notification-channels/${id}/test`, {
      method: 'POST',
    });
  }
}

export const api = new ApiClient();
