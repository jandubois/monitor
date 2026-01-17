import type {
  ProbeType,
  ProbeConfig,
  ProbeResult,
  NotificationChannel,
  SystemStatus,
  ResultStats,
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

  async getProbeTypes(): Promise<ProbeType[]> {
    return this.request('/probe-types');
  }

  async getProbeConfigs(): Promise<ProbeConfig[]> {
    return this.request('/probe-configs');
  }

  async getProbeConfig(id: number): Promise<ProbeConfig> {
    return this.request(`/probe-configs/${id}`);
  }

  async createProbeConfig(config: Omit<ProbeConfig, 'id' | 'probe_type_name' | 'created_at' | 'updated_at'>): Promise<{ id: number }> {
    return this.request('/probe-configs', {
      method: 'POST',
      body: JSON.stringify(config),
    });
  }

  async updateProbeConfig(id: number, config: Partial<ProbeConfig>): Promise<void> {
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

  async getResults(params?: { config_id?: number; status?: string; since?: string; limit?: number }): Promise<ProbeResult[]> {
    const searchParams = new URLSearchParams();
    if (params?.config_id) searchParams.set('config_id', String(params.config_id));
    if (params?.status) searchParams.set('status', params.status);
    if (params?.since) searchParams.set('since', params.since);
    if (params?.limit) searchParams.set('limit', String(params.limit));
    const query = searchParams.toString();
    return this.request(`/results${query ? `?${query}` : ''}`);
  }

  async getProbeResults(configId: number): Promise<ProbeResult[]> {
    return this.request(`/results/${configId}`);
  }

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
