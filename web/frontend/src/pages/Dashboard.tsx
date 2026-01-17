import { useQuery } from '@tanstack/react-query';
import { api } from '../api/client';
import { ProbeCard } from '../components/ProbeCard';
import type { ProbeConfig } from '../api/types';

interface DashboardProps {
  onProbeClick: (config: ProbeConfig) => void;
}

export function Dashboard({ onProbeClick }: DashboardProps) {
  const { data: status } = useQuery({
    queryKey: ['status'],
    queryFn: () => api.getStatus(),
    refetchInterval: 10000,
  });

  const { data: stats } = useQuery({
    queryKey: ['stats'],
    queryFn: () => api.getResultStats(),
    refetchInterval: 30000,
  });

  const { data: configs, isLoading } = useQuery({
    queryKey: ['probeConfigs'],
    queryFn: () => api.getProbeConfigs(),
    refetchInterval: 30000,
  });

  const sortedConfigs = configs?.sort((a, b) => {
    const statusOrder = { critical: 0, warning: 1, unknown: 2, ok: 3 };
    const aOrder = a.last_status ? statusOrder[a.last_status] : 4;
    const bOrder = b.last_status ? statusOrder[b.last_status] : 4;
    return aOrder - bOrder;
  });

  return (
    <div className="p-6">
      <div className="mb-6">
        <h1 className="text-2xl font-bold text-gray-900">Monitor Dashboard</h1>
      </div>

      <div className="grid grid-cols-1 md:grid-cols-4 gap-4 mb-6">
        <div className="bg-white rounded-lg shadow p-4 border border-gray-200">
          <div className="text-sm text-gray-500">Watcher Status</div>
          <div className={`text-lg font-semibold ${status?.watcher_healthy ? 'text-green-600' : 'text-red-600'}`}>
            {status?.watcher_healthy ? 'Healthy' : 'Unhealthy'}
          </div>
        </div>

        <div className="bg-white rounded-lg shadow p-4 border border-gray-200">
          <div className="text-sm text-gray-500">Active Probes</div>
          <div className="text-lg font-semibold text-gray-900">
            {stats?.enabled_configs ?? '-'} / {stats?.total_configs ?? '-'}
          </div>
        </div>

        <div className="bg-white rounded-lg shadow p-4 border border-gray-200">
          <div className="text-sm text-gray-500">Status Summary</div>
          <div className="flex gap-2 mt-1">
            <span className="text-green-600">{stats?.status_counts?.ok ?? 0} OK</span>
            <span className="text-yellow-600">{stats?.status_counts?.warning ?? 0} Warn</span>
            <span className="text-red-600">{stats?.status_counts?.critical ?? 0} Crit</span>
          </div>
        </div>

        <div className="bg-white rounded-lg shadow p-4 border border-gray-200">
          <div className="text-sm text-gray-500">Recent Failures</div>
          <div className={`text-lg font-semibold ${(status?.recent_failures ?? 0) > 0 ? 'text-red-600' : 'text-gray-900'}`}>
            {status?.recent_failures ?? 0}
          </div>
        </div>
      </div>

      {isLoading ? (
        <div className="text-center py-12 text-gray-500">Loading probes...</div>
      ) : sortedConfigs?.length === 0 ? (
        <div className="text-center py-12 text-gray-500">
          No probes configured yet.
        </div>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
          {sortedConfigs?.map((config) => (
            <ProbeCard
              key={config.id}
              config={config}
              onClick={() => onProbeClick(config)}
            />
          ))}
        </div>
      )}
    </div>
  );
}
