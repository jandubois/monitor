import { useState, useEffect } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '../api/client';
import { ProbeRow } from '../components/ProbeRow';
import { ProbeConfigForm } from '../components/ProbeConfigForm';
import type { ProbeConfig, ProbeResult } from '../api/types';

const COLLAPSED_GROUPS_KEY = 'dashboard-collapsed-groups';

function formatRelativeTime(timestamp: string): string {
  const diff = Date.now() - new Date(timestamp).getTime();
  if (diff < 60000) return 'now';
  if (diff < 3600000) return `${Math.floor(diff / 60000)}m`;
  if (diff < 86400000) return `${Math.floor(diff / 3600000)}h`;
  return `${Math.floor(diff / 86400000)}d`;
}

interface DashboardProps {
  onProbeClick: (config: ProbeConfig) => void;
  onConfigClick: () => void;
  onFailuresClick: () => void;
}

export function Dashboard({ onProbeClick, onConfigClick, onFailuresClick }: DashboardProps) {
  const [editingConfig, setEditingConfig] = useState<ProbeConfig | null>(null);
  const [keywordFilter, setKeywordFilter] = useState('');
  const [runningProbes, setRunningProbes] = useState<Set<number>>(new Set());
  const [collapsedGroups, setCollapsedGroups] = useState<Set<string>>(() => {
    try {
      const saved = localStorage.getItem(COLLAPSED_GROUPS_KEY);
      return saved ? new Set(JSON.parse(saved)) : new Set();
    } catch {
      return new Set();
    }
  });
  const queryClient = useQueryClient();

  useEffect(() => {
    localStorage.setItem(COLLAPSED_GROUPS_KEY, JSON.stringify([...collapsedGroups]));
  }, [collapsedGroups]);

  const toggleGroup = (group: string) => {
    setCollapsedGroups(prev => {
      const next = new Set(prev);
      if (next.has(group)) {
        next.delete(group);
      } else {
        next.add(group);
      }
      return next;
    });
  };

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

  // Mark a probe as running and poll for completion
  const trackRunningProbe = (id: number) => {
    setRunningProbes(prev => new Set(prev).add(id));

    const pollForResult = async () => {
      for (let i = 0; i < 30; i++) {
        await new Promise(r => setTimeout(r, 1000));
        await queryClient.invalidateQueries({ queryKey: ['probeConfigs'] });
        const configs = queryClient.getQueryData<ProbeConfig[]>(['probeConfigs']);
        const config = configs?.find(c => c.id === id);
        if (config?.last_executed_at) {
          const lastRun = new Date(config.last_executed_at).getTime();
          if (Date.now() - lastRun < 5000) {
            setRunningProbes(prev => {
              const next = new Set(prev);
              next.delete(id);
              return next;
            });
            return;
          }
        }
      }
      setRunningProbes(prev => {
        const next = new Set(prev);
        next.delete(id);
        return next;
      });
    };
    pollForResult();
  };

  const rerunMutation = useMutation({
    mutationFn: (id: number) => api.triggerProbe(id),
    onMutate: (id) => {
      trackRunningProbe(id);
    },
    onError: (_error, id) => {
      setRunningProbes(prev => {
        const next = new Set(prev);
        next.delete(id);
        return next;
      });
    },
  });

  const pauseToggleMutation = useMutation({
    mutationFn: ({ id, enabled }: { id: number; enabled: boolean }) =>
      api.setProbeEnabled(id, enabled),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['probeConfigs'] });
    },
  });

  // Filter by keyword, then sort, then group
  const filteredConfigs = configs?.filter(config => {
    if (!keywordFilter.trim()) return true;
    const filterLower = keywordFilter.toLowerCase();
    // Match against keywords array if present
    if (config.keywords?.some(k => k.toLowerCase().includes(filterLower))) {
      return true;
    }
    // Also match against name and probe type
    return config.name.toLowerCase().includes(filterLower) ||
           config.probe_type_name?.toLowerCase().includes(filterLower);
  });

  const sortedConfigs = filteredConfigs?.sort((a, b) => {
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

      <div className="mb-4">
        <input
          type="text"
          value={keywordFilter}
          onChange={(e) => setKeywordFilter(e.target.value)}
          placeholder="Filter by keyword..."
          className="w-full max-w-md px-3 py-2 border border-gray-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent"
        />
      </div>

      {isLoading ? (
        <div className="text-center py-12 text-gray-500">Loading probes...</div>
      ) : sortedConfigs?.length === 0 ? (
        <div className="text-center py-12 text-gray-500">
          {keywordFilter ? 'No probes match this filter.' : 'No probes configured yet.'}
        </div>
      ) : (
        <div className="space-y-4">
          {groups.map((group) => {
            const isCollapsed = collapsedGroups.has(group);
            const groupProbes = groupedConfigs![group];
            const statusCounts = groupProbes.reduce(
              (acc, c) => {
                const status = c.last_status || 'unknown';
                acc[status] = (acc[status] || 0) + 1;
                return acc;
              },
              {} as Record<string, number>
            );
            return (
              <div key={group} className="bg-white rounded-lg shadow border border-gray-200 overflow-hidden">
                <button
                  onClick={() => toggleGroup(group)}
                  className="w-full px-4 py-3 flex items-center justify-between bg-gray-50 hover:bg-gray-100 transition-colors"
                >
                  <div className="flex items-center gap-3">
                    <h2 className="text-lg font-semibold text-gray-700">{group}</h2>
                    <div className="flex gap-2 text-xs">
                      {statusCounts.ok > 0 && (
                        <span className="text-green-600">{statusCounts.ok} ok</span>
                      )}
                      {statusCounts.warning > 0 && (
                        <span className="text-yellow-600">{statusCounts.warning} warn</span>
                      )}
                      {statusCounts.critical > 0 && (
                        <span className="text-red-600">{statusCounts.critical} crit</span>
                      )}
                      {statusCounts.unknown > 0 && (
                        <span className="text-gray-500">{statusCounts.unknown} unknown</span>
                      )}
                    </div>
                  </div>
                  <svg
                    className={`w-5 h-5 text-gray-500 transition-transform ${isCollapsed ? '' : 'rotate-180'}`}
                    fill="none"
                    stroke="currentColor"
                    viewBox="0 0 24 24"
                  >
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 9l-7 7-7-7" />
                  </svg>
                </button>
                {!isCollapsed && (
                  <div className="divide-y divide-gray-100">
                    {groupedConfigs![group].map((config) => (
                      <ProbeRow
                        key={config.id}
                        config={config}
                        isRunning={runningProbes.has(config.id)}
                        onClick={() => onProbeClick(config)}
                        onEdit={() => setEditingConfig(config)}
                        onRerun={() => rerunMutation.mutate(config.id)}
                        onPauseToggle={() => pauseToggleMutation.mutate({
                          id: config.id,
                          enabled: !config.enabled,
                        })}
                      />
                    ))}
                  </div>
                )}
              </div>
            );
          })}
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
          onRerun={trackRunningProbe}
        />
      )}
    </div>
  );
}
