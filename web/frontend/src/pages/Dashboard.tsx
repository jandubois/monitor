import { useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '../api/client';
import { ProbeCard } from '../components/ProbeCard';
import { ProbeConfigForm } from '../components/ProbeConfigForm';
import type { ProbeConfig, ProbeResult } from '../api/types';

interface DashboardProps {
  onProbeClick: (config: ProbeConfig) => void;
  onConfigClick: () => void;
  onFailuresClick: () => void;
}

export function Dashboard({ onProbeClick, onConfigClick, onFailuresClick }: DashboardProps) {
  const [editingConfig, setEditingConfig] = useState<ProbeConfig | null>(null);
  const queryClient = useQueryClient();

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

  const { data: recentFailures } = useQuery({
    queryKey: ['recentFailures'],
    queryFn: () => api.getResults({ status: ['critical', 'unknown', 'warning'], limit: 10 }),
    refetchInterval: 10000,
  });

  const { data: watchers } = useQuery({
    queryKey: ['watchers'],
    queryFn: () => api.getWatchers(),
    enabled: !!editingConfig,
  });

  const { data: probeTypes } = useQuery({
    queryKey: ['probeTypes'],
    queryFn: () => api.getProbeTypes(),
    enabled: !!editingConfig,
  });

  const rerunMutation = useMutation({
    mutationFn: (id: number) => api.triggerProbe(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['probeConfigs'] });
    },
  });

  const formatRelativeTime = (timestamp: string) => {
    const diff = Date.now() - new Date(timestamp).getTime();
    if (diff < 60000) return 'now';
    if (diff < 3600000) return `${Math.floor(diff / 60000)}m`;
    if (diff < 86400000) return `${Math.floor(diff / 3600000)}h`;
    return `${Math.floor(diff / 86400000)}d`;
  };

  const sortedConfigs = configs?.sort((a, b) => {
    const statusOrder = { critical: 0, warning: 1, unknown: 2, ok: 3 };
    const aOrder = a.last_status ? statusOrder[a.last_status] : 4;
    const bOrder = b.last_status ? statusOrder[b.last_status] : 4;
    return aOrder - bOrder;
  });

  // Group configs by group_path
  const groupedConfigs = sortedConfigs?.reduce((acc, config) => {
    const group = config.group_path || 'Uncategorized';
    if (!acc[group]) acc[group] = [];
    acc[group].push(config);
    return acc;
  }, {} as Record<string, ProbeConfig[]>);

  const groups = groupedConfigs ? Object.keys(groupedConfigs).sort() : [];

  return (
    <div className="p-6">
      <div className="mb-6 flex items-center justify-between">
        <h1 className="text-2xl font-bold text-gray-900">Monitor Dashboard</h1>
        <button
          onClick={onConfigClick}
          className="px-4 py-2 bg-gray-800 text-white rounded hover:bg-gray-700"
        >
          Configure
        </button>
      </div>

      <div className="grid grid-cols-1 md:grid-cols-4 gap-4 mb-6">
        <div className="bg-white rounded-lg shadow p-4 border border-gray-200">
          <div className="text-sm text-gray-500">Watchers</div>
          <div className="mt-1">
            {status?.watchers?.length ? (
              <div className="flex flex-wrap gap-2">
                {status.watchers.map((w) => (
                  <span
                    key={w.name}
                    className={`inline-flex items-center px-2 py-0.5 rounded text-xs font-medium ${
                      w.healthy
                        ? 'bg-green-100 text-green-800'
                        : 'bg-red-100 text-red-800'
                    }`}
                    title={w.version ? `v${w.version}` : undefined}
                  >
                    {w.name}
                  </span>
                ))}
              </div>
            ) : (
              <span className="text-gray-400">No watchers</span>
            )}
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

        <div
          className="bg-white rounded-lg shadow p-4 border border-gray-200 cursor-pointer hover:bg-gray-50"
          onClick={onFailuresClick}
        >
          <div className="text-sm text-gray-500 flex items-center justify-between">
            <span>Recent Failures</span>
            <span className="text-xs text-gray-400">View all →</span>
          </div>
          {recentFailures && recentFailures.length > 0 ? (
            <div className="mt-2 space-y-1 max-h-32 overflow-y-auto">
              {recentFailures.slice(0, 5).map((f: ProbeResult) => (
                <div key={f.id} className="flex items-center gap-2 text-sm">
                  <span
                    className={`w-2 h-2 rounded-full flex-shrink-0 ${
                      f.status === 'critical' ? 'bg-red-500' :
                      f.status === 'warning' ? 'bg-yellow-500' : 'bg-gray-500'
                    }`}
                  />
                  <span className="font-medium text-gray-700 truncate">{f.config_name}</span>
                  <span className="text-gray-400 text-xs flex-shrink-0">{formatRelativeTime(f.executed_at)}</span>
                </div>
              ))}
              {recentFailures.length > 5 && (
                <div className="text-xs text-gray-400 pl-4">
                  +{recentFailures.length - 5} more
                </div>
              )}
            </div>
          ) : (
            <div className="text-lg font-semibold text-green-600 mt-1">None</div>
          )}
        </div>
      </div>

      {isLoading ? (
        <div className="text-center py-12 text-gray-500">Loading probes...</div>
      ) : sortedConfigs?.length === 0 ? (
        <div className="text-center py-12 text-gray-500">
          No probes configured yet.
        </div>
      ) : (
        <div className="space-y-6">
          {groups.map((group) => (
            <div key={group}>
              <h2 className="text-lg font-semibold text-gray-700 mb-3">{group}</h2>
              <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
                {groupedConfigs![group].map((config) => (
                  <ProbeCard
                    key={config.id}
                    config={config}
                    onStatusClick={() => onProbeClick(config)}
                    onEdit={() => setEditingConfig(config)}
                    onRerun={() => rerunMutation.mutate(config.id)}
                  />
                ))}
              </div>
            </div>
          ))}
        </div>
      )}

      {editingConfig && watchers && probeTypes && (
        <ProbeConfigForm
          probeTypes={probeTypes}
          watchers={watchers}
          editingConfig={editingConfig}
          onClose={() => setEditingConfig(null)}
          onSaved={() => {
            setEditingConfig(null);
            queryClient.invalidateQueries({ queryKey: ['probeConfigs'] });
          }}
        />
      )}
    </div>
  );
}
