import { useState, useEffect, useMemo } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '../api/client';
import { ProbeRow } from '../components/ProbeRow';
import { ProbeConfigForm } from '../components/ProbeConfigForm';
import { WatchersGroup } from '../components/WatchersGroup';
import { COLOR_PALETTE, getColorByIndex } from '../utils/keywordColors';
import type { ProbeConfig } from '../api/types';

const COLLAPSED_GROUPS_KEY = 'dashboard-collapsed-groups';

interface DashboardProps {
  onProbeClick: (config: ProbeConfig) => void;
  onFailuresClick: () => void;
}

export function Dashboard({ onProbeClick, onFailuresClick }: DashboardProps) {
  const [showForm, setShowForm] = useState(false);
  const [editingConfig, setEditingConfig] = useState<ProbeConfig | null>(null);
  const [keywordFilter, setKeywordFilter] = useState('');
  const [showManageModal, setShowManageModal] = useState<'groups' | 'keywords' | null>(null);
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

  // Update browser title when server name is loaded
  useEffect(() => {
    if (status?.server_name) {
      document.title = `Monitor Dashboard on ${status.server_name}`;
    }
  }, [status?.server_name]);

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
    enabled: showForm,
  });

  const { data: keywordColors = {} } = useQuery({
    queryKey: ['keywordColors'],
    queryFn: () => api.getKeywordColors(),
  });

  // Mark a probe as running and poll for completion
  const trackRunningProbe = (id: number) => {
    setRunningProbes(prev => new Set(prev).add(id));

    const pollForResult = async () => {
      const startTime = Date.now();
      for (let i = 0; i < 30; i++) {
        await new Promise(r => setTimeout(r, 1000));
        await queryClient.refetchQueries({ queryKey: ['probeConfigs'] });
        const configs = queryClient.getQueryData<ProbeConfig[]>(['probeConfigs']);
        const config = configs?.find(c => c.id === id);
        if (config?.last_executed_at) {
          const lastRun = new Date(config.last_executed_at).getTime();
          if (lastRun > startTime) {
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
    onMutate: ({ id, enabled }) => {
      if (enabled) {
        trackRunningProbe(id);
      }
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['probeConfigs'] });
    },
    onError: (_error, { id, enabled }) => {
      if (enabled) {
        setRunningProbes(prev => {
          const next = new Set(prev);
          next.delete(id);
          return next;
        });
      }
    },
  });

  const deleteMutation = useMutation({
    mutationFn: (id: number) => api.deleteProbeConfig(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['probeConfigs'] });
    },
  });

  const renameGroupMutation = useMutation({
    mutationFn: ({ oldName, newName }: { oldName: string; newName: string }) =>
      api.renameGroup(oldName, newName),
    onSuccess: (_data, { oldName, newName }) => {
      queryClient.invalidateQueries({ queryKey: ['probeConfigs'] });
      // Update collapsed state if group was renamed
      if (collapsedGroups.has(oldName)) {
        setCollapsedGroups(prev => {
          const next = new Set(prev);
          next.delete(oldName);
          if (newName) next.add(newName);
          return next;
        });
      }
    },
  });

  const renameKeywordMutation = useMutation({
    mutationFn: ({ oldName, newName }: { oldName: string; newName: string }) =>
      api.renameKeyword(oldName, newName),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['probeConfigs'] });
      queryClient.invalidateQueries({ queryKey: ['keywordColors'] });
    },
  });

  const setKeywordColorMutation = useMutation({
    mutationFn: ({ keyword, colorIndex }: { keyword: string; colorIndex: number }) =>
      api.setKeywordColor(keyword, colorIndex),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['keywordColors'] });
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
        <h1 className="text-2xl font-bold text-gray-900">
          Monitor Dashboard{status?.server_name ? ` on ${status.server_name}` : ''}
        </h1>
        <div className="flex items-center gap-4">
          {recentFailures && recentFailures.length > 0 ? (
            <button
              onClick={onFailuresClick}
              className="flex items-center gap-2 px-3 py-2 text-red-600 hover:text-red-800 hover:bg-red-50 rounded"
            >
              <span className="w-2 h-2 rounded-full bg-red-500" />
              <span className="font-medium">{recentFailures.length} failure{recentFailures.length > 1 ? 's' : ''}</span>
            </button>
          ) : (
            <span className="text-green-600 text-sm">No failures</span>
          )}
          <span className="text-gray-300">|</span>
          <button
            onClick={() => setShowManageModal('groups')}
            className="text-gray-500 hover:text-gray-700 text-sm"
          >
            Groups
          </button>
          <button
            onClick={() => setShowManageModal('keywords')}
            className="text-gray-500 hover:text-gray-700 text-sm"
          >
            Keywords
          </button>
          <button
            onClick={() => { setEditingConfig(null); setShowForm(true); }}
            className="px-4 py-2 bg-green-600 text-white rounded hover:bg-green-700"
          >
            Add Probe
          </button>
        </div>
      </div>

      <div className="space-y-4">
        <WatchersGroup />

        <div>
          <input
            type="text"
            value={keywordFilter}
            onChange={(e) => setKeywordFilter(e.target.value)}
            placeholder="Filter by keyword..."
            className="w-full px-3 py-2 border border-gray-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent"
          />
        </div>

        {isLoading ? (
          <div className="text-center py-12 text-gray-500">Loading probes...</div>
        ) : sortedConfigs?.length === 0 ? (
          <div className="text-center py-12 text-gray-500">
            {keywordFilter ? 'No probes match this filter.' : 'No probes configured yet.'}
          </div>
        ) : (
          <>
          {groups.map((group) => {
            const isCollapsed = collapsedGroups.has(group);
            const groupProbes = groupedConfigs![group];
            const pausedCount = groupProbes.filter(c => !c.enabled).length;
            const statusCounts = groupProbes.reduce(
              (acc, c) => {
                if (!c.enabled) return acc; // Don't count paused probes in status
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
                  className="w-full px-4 py-3 flex items-center gap-3 bg-gray-200 hover:bg-gray-300 transition-colors"
                >
                  <svg
                    className={`w-5 h-5 text-gray-500 transition-transform ${isCollapsed ? '' : 'rotate-90'}`}
                    fill="none"
                    stroke="currentColor"
                    viewBox="0 0 24 24"
                  >
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 5l7 7-7 7" />
                  </svg>
                  <h2 className="text-lg font-semibold text-gray-700">{group}</h2>
                  <div className="flex gap-2 text-xs">
                    {statusCounts.ok > 0 && (
                      <span className="text-green-600">{statusCounts.ok} ok</span>
                    )}
                    {statusCounts.warning > 0 && (
                      <span className="text-yellow-600">{statusCounts.warning} warning</span>
                    )}
                    {statusCounts.critical > 0 && (
                      <span className="text-red-600">{statusCounts.critical} critical</span>
                    )}
                    {statusCounts.unknown > 0 && (
                      <span className="text-gray-500">{statusCounts.unknown} unknown</span>
                    )}
                    {pausedCount > 0 && (
                      <span className="text-gray-400">{pausedCount} paused</span>
                    )}
                  </div>
                </button>
                {!isCollapsed && (
                  <div className="divide-y divide-gray-300">
                    {groupedConfigs![group].map((config) => (
                      <ProbeRow
                        key={config.id}
                        config={config}
                        keywordColors={keywordColors}
                        isRunning={runningProbes.has(config.id)}
                        onClick={() => onProbeClick(config)}
                        onEdit={() => { setEditingConfig(config); setShowForm(true); }}
                        onRerun={() => rerunMutation.mutate(config.id)}
                        onPauseToggle={() => pauseToggleMutation.mutate({
                          id: config.id,
                          enabled: !config.enabled,
                        })}
                        onDelete={() => {
                          if (confirm(`Delete probe "${config.name}"?`)) {
                            deleteMutation.mutate(config.id);
                          }
                        }}
                      />
                    ))}
                  </div>
                )}
              </div>
            );
          })}
          </>
        )}
      </div>

      {showForm && watchers && (
        <ProbeConfigForm
          watchers={watchers}
          editingConfig={editingConfig}
          keywordColors={keywordColors}
          onClose={() => { setShowForm(false); setEditingConfig(null); }}
          onSaved={() => {
            setShowForm(false);
            setEditingConfig(null);
            queryClient.invalidateQueries({ queryKey: ['probeConfigs'] });
            queryClient.invalidateQueries({ queryKey: ['keywordColors'] });
          }}
          onRerun={trackRunningProbe}
        />
      )}

      {showManageModal && (
        <ManageModal
          type={showManageModal}
          configs={configs || []}
          keywordColors={keywordColors}
          onRename={(oldName, newName) => {
            if (showManageModal === 'groups') {
              renameGroupMutation.mutate({ oldName, newName });
            } else {
              renameKeywordMutation.mutate({ oldName, newName });
            }
          }}
          onSetColor={(keyword, colorIndex) => {
            setKeywordColorMutation.mutate({ keyword, colorIndex });
          }}
          onClose={() => setShowManageModal(null)}
        />
      )}
    </div>
  );
}

interface ManageModalProps {
  type: 'groups' | 'keywords';
  configs: ProbeConfig[];
  keywordColors: Record<string, number>;
  onRename: (oldName: string, newName: string) => void;
  onSetColor?: (keyword: string, colorIndex: number) => void;
  onClose: () => void;
}

function ManageModal({ type, configs, keywordColors, onRename, onSetColor, onClose }: ManageModalProps) {
  const [editingItem, setEditingItem] = useState<string | null>(null);
  const [editValue, setEditValue] = useState('');

  // Extract items with counts
  const items = useMemo(() => {
    const counts = new Map<string, number>();
    configs.forEach(config => {
      if (type === 'groups') {
        const group = config.group_path || '';
        if (group) {
          counts.set(group, (counts.get(group) || 0) + 1);
        }
      } else {
        config.keywords?.forEach(keyword => {
          counts.set(keyword, (counts.get(keyword) || 0) + 1);
        });
      }
    });
    return Array.from(counts.entries())
      .sort((a, b) => a[0].localeCompare(b[0]))
      .map(([name, count]) => ({ name, count }));
  }, [configs, type]);

  const handleSave = () => {
    if (editingItem && editValue !== editingItem) {
      onRename(editingItem, editValue);
    }
    setEditingItem(null);
  };

  const handleDelete = (name: string) => {
    if (confirm(`Remove "${name}" from all probes?`)) {
      onRename(name, '');
    }
  };

  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        if (editingItem) {
          setEditingItem(null);
        } else {
          onClose();
        }
      }
    };
    document.addEventListener('keydown', handleKeyDown);
    return () => document.removeEventListener('keydown', handleKeyDown);
  }, [editingItem, onClose]);

  return (
    <div className="fixed inset-0 bg-black/50 flex items-center justify-center p-4 z-50">
      <div className="bg-white rounded-lg shadow-xl w-full max-w-md max-h-[80vh] overflow-hidden flex flex-col">
        <div className="p-4 border-b flex items-center justify-between">
          <h3 className="text-lg font-semibold">
            Manage {type === 'groups' ? 'Groups' : 'Keywords'}
          </h3>
          <button onClick={onClose} className="text-gray-400 hover:text-gray-600">
            <svg className="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M6 18L18 6M6 6l12 12" />
            </svg>
          </button>
        </div>
        <div className="flex-1 overflow-y-auto p-4">
          {items.length === 0 ? (
            <p className="text-gray-500 text-center py-8">
              No {type} defined yet.
            </p>
          ) : (
            <div className="space-y-2">
              {items.map(({ name, count }) => {
                const colorIndex = keywordColors[name] ?? 0;
                const color = getColorByIndex(colorIndex);
                return (
                  <div key={name} className="flex items-center gap-2 p-2 rounded hover:bg-gray-50">
                    {type === 'keywords' && (
                      <button
                        onClick={() => onSetColor?.(name, (colorIndex + 1) % COLOR_PALETTE.length)}
                        className="w-6 h-6 rounded-full border-2 border-white shadow-sm flex-shrink-0"
                        style={{ backgroundColor: color.hex }}
                        title="Click to change color"
                      />
                    )}
                    {editingItem === name ? (
                      <input
                        type="text"
                        value={editValue}
                        onChange={(e) => setEditValue(e.target.value)}
                        onBlur={handleSave}
                        onKeyDown={(e) => {
                          if (e.key === 'Enter') handleSave();
                          if (e.key === 'Escape') setEditingItem(null);
                        }}
                        className="flex-1 px-2 py-1 border rounded"
                        autoFocus
                      />
                    ) : (
                      <span
                        className="flex-1 cursor-pointer hover:text-blue-600"
                        onClick={() => {
                          setEditingItem(name);
                          setEditValue(name);
                        }}
                      >
                        {name}
                      </span>
                    )}
                    <span className="text-xs text-gray-400">{count} probe{count !== 1 ? 's' : ''}</span>
                    <button
                      onClick={() => handleDelete(name)}
                      className="text-gray-400 hover:text-red-600 p-1"
                      title="Remove from all probes"
                    >
                      <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
                      </svg>
                    </button>
                  </div>
                );
              })}
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
