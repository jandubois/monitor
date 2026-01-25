import { useState, useEffect } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '../api/client';
import { ProbeConfigForm } from '../components/ProbeConfigForm';
import type { ProbeConfig, WatcherEvent } from '../api/types';

interface ConfigProps {
  onBack: () => void;
}

export function Config({ onBack }: ConfigProps) {
  const queryClient = useQueryClient();
  const [showForm, setShowForm] = useState(false);
  const [editingConfig, setEditingConfig] = useState<ProbeConfig | null>(null);
  const [initialProbeTypeId, setInitialProbeTypeId] = useState<number | undefined>();
  const [expandedWatcher, setExpandedWatcher] = useState<number | null>(null);

  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape' && !showForm) {
        onBack();
      }
    };
    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [onBack, showForm]);

  const { data: watchers } = useQuery({
    queryKey: ['watchers'],
    queryFn: () => api.getWatchers(),
  });

  // Fetch unacknowledged events for badge counts
  const { data: unacknowledgedEvents } = useQuery({
    queryKey: ['watcherEvents', 'unacknowledged'],
    queryFn: () => api.getWatcherEventsAll({ unacknowledged: true }),
    refetchInterval: 30000,
  });

  // Group events by watcher for badge counts
  const eventCountByWatcher = unacknowledgedEvents?.reduce((acc, e) => {
    acc[e.watcher_id] = (acc[e.watcher_id] || 0) + 1;
    return acc;
  }, {} as Record<number, number>) || {};

  // Fetch events for expanded watcher
  const { data: watcherEvents } = useQuery({
    queryKey: ['watcherEvents', expandedWatcher],
    queryFn: () => expandedWatcher ? api.getWatcherEvents(expandedWatcher) : Promise.resolve([]),
    enabled: !!expandedWatcher,
  });

  const { data: probeTypes, isLoading: typesLoading } = useQuery({
    queryKey: ['probeTypes'],
    queryFn: () => api.getProbeTypes(),
  });

  const { data: configs, isLoading: configsLoading } = useQuery({
    queryKey: ['probeConfigs'],
    queryFn: () => api.getProbeConfigs(),
  });

  const discoverMutation = useMutation({
    mutationFn: () => api.discoverProbeTypes(),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['probeTypes'] });
    },
  });

  const deleteMutation = useMutation({
    mutationFn: (id: number) => api.deleteProbeConfig(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['probeConfigs'] });
    },
  });

  const deleteWatcherMutation = useMutation({
    mutationFn: (id: number) => api.deleteWatcher(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['watchers'] });
      queryClient.invalidateQueries({ queryKey: ['probeConfigs'] });
    },
  });

  const pauseWatcherMutation = useMutation({
    mutationFn: ({ id, paused }: { id: number; paused: boolean }) => api.setWatcherPaused(id, paused),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['watchers'] });
    },
  });

  const acknowledgeEventMutation = useMutation({
    mutationFn: (id: number) => api.acknowledgeWatcherEvent(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['watcherEvents'] });
    },
  });

  return (
    <div className="p-6">
      <button
        onClick={onBack}
        className="mb-4 text-blue-600 hover:text-blue-800 flex items-center gap-1"
      >
        &larr; Back to Dashboard
      </button>

      <h1 className="text-2xl font-bold text-gray-900 mb-6">Configuration</h1>

      {/* Watchers Section */}
      <div className="bg-white rounded-lg shadow p-6 mb-6 border border-gray-200">
        <h2 className="text-lg font-semibold mb-4">Watchers</h2>
        {watchers?.length === 0 ? (
          <p className="text-gray-500">No watchers registered. Start a watcher to register it.</p>
        ) : (
          <div className="grid gap-3">
            {watchers?.map((w) => (
              <div key={w.id} className="border rounded bg-gray-50">
                <div className="p-3 flex items-center justify-between">
                  <div className="flex items-center gap-2">
                    <button
                      onClick={() => setExpandedWatcher(expandedWatcher === w.id ? null : w.id)}
                      className="text-gray-400 hover:text-gray-600"
                    >
                      <svg
                        className={`w-4 h-4 transition-transform ${expandedWatcher === w.id ? 'rotate-90' : ''}`}
                        fill="none"
                        stroke="currentColor"
                        viewBox="0 0 24 24"
                      >
                        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 5l7 7-7 7" />
                      </svg>
                    </button>
                    <span className="font-medium">{w.name}</span>
                    {w.version && <span className="text-gray-400 text-sm">v{w.version}</span>}
                    <span className={`text-xs px-2 py-0.5 rounded ${w.healthy ? 'bg-green-100 text-green-700' : 'bg-red-100 text-red-700'}`}>
                      {w.healthy ? 'healthy' : 'unhealthy'}
                    </span>
                    {!w.approved && (
                      <span className="text-xs px-2 py-0.5 rounded bg-orange-100 text-orange-700">
                        pending approval
                      </span>
                    )}
                    {w.approved && w.paused && (
                      <span className="text-xs px-2 py-0.5 rounded bg-yellow-100 text-yellow-700">
                        paused
                      </span>
                    )}
                    {eventCountByWatcher[w.id] > 0 && (
                      <span className="text-xs px-2 py-0.5 rounded bg-red-100 text-red-700 font-medium">
                        {eventCountByWatcher[w.id]} event{eventCountByWatcher[w.id] > 1 ? 's' : ''}
                      </span>
                    )}
                    <span className="text-sm text-gray-500">
                      {w.probe_type_count} types, {w.config_count} configs
                    </span>
                  </div>
                  <div className="flex gap-2">
                    <button
                      onClick={() => pauseWatcherMutation.mutate({ id: w.id, paused: !w.paused })}
                      className={`text-sm px-2 py-1 rounded ${w.paused ? 'text-green-600 hover:text-green-800' : 'text-yellow-600 hover:text-yellow-800'}`}
                      title={!w.approved ? 'Approve watcher' : (w.paused ? 'Resume notifications' : 'Pause notifications')}
                    >
                      {!w.approved ? 'Approve' : (w.paused ? 'Resume' : 'Pause')}
                    </button>
                    <button
                      onClick={() => {
                        if (confirm(`Delete watcher "${w.name}" and all its probe data?`)) {
                          deleteWatcherMutation.mutate(w.id);
                        }
                      }}
                      className="text-red-600 hover:text-red-800 text-sm"
                    >
                      Delete
                    </button>
                  </div>
                </div>
                {expandedWatcher === w.id && (
                  <div className="border-t bg-white p-3">
                    <h4 className="text-sm font-medium text-gray-700 mb-2">Recent Events</h4>
                    {watcherEvents?.length === 0 ? (
                      <p className="text-sm text-gray-500">No events recorded</p>
                    ) : (
                      <div className="space-y-2">
                        {watcherEvents?.slice(0, 10).map((event: WatcherEvent) => (
                          <div
                            key={event.id}
                            className={`text-sm p-2 rounded border ${
                              event.acknowledged
                                ? 'bg-gray-50 border-gray-200'
                                : event.severity === 'error'
                                  ? 'bg-red-50 border-red-200'
                                  : event.severity === 'warning'
                                    ? 'bg-yellow-50 border-yellow-200'
                                    : 'bg-blue-50 border-blue-200'
                            }`}
                          >
                            <div className="flex items-center justify-between">
                              <div className="flex items-center gap-2">
                                <span className={`text-xs px-1.5 py-0.5 rounded font-medium ${
                                  event.severity === 'error'
                                    ? 'bg-red-100 text-red-700'
                                    : event.severity === 'warning'
                                      ? 'bg-yellow-100 text-yellow-700'
                                      : 'bg-blue-100 text-blue-700'
                                }`}>
                                  {event.event_type.replace(/_/g, ' ')}
                                </span>
                                <span className="text-gray-600">{event.summary}</span>
                              </div>
                              <div className="flex items-center gap-2">
                                <span className="text-xs text-gray-400">
                                  {new Date(event.timestamp).toLocaleString()}
                                </span>
                                {!event.acknowledged && (
                                  <button
                                    onClick={() => acknowledgeEventMutation.mutate(event.id)}
                                    className="text-xs text-blue-600 hover:text-blue-800"
                                  >
                                    Acknowledge
                                  </button>
                                )}
                              </div>
                            </div>
                          </div>
                        ))}
                      </div>
                    )}
                  </div>
                )}
              </div>
            ))}
          </div>
        )}
      </div>

      {/* Probe Types Section */}
      <div className="bg-white rounded-lg shadow p-6 mb-6 border border-gray-200">
        <div className="flex items-center justify-between mb-4">
          <h2 className="text-lg font-semibold">Probe Types</h2>
          <button
            onClick={() => discoverMutation.mutate()}
            disabled={discoverMutation.isPending}
            className="px-4 py-2 bg-blue-600 text-white rounded hover:bg-blue-700 disabled:opacity-50"
          >
            {discoverMutation.isPending ? 'Refreshing...' : 'Refresh'}
          </button>
        </div>

        {typesLoading ? (
          <p className="text-gray-500">Loading probe types...</p>
        ) : probeTypes?.length === 0 ? (
          <p className="text-gray-500">No probe types registered. Watchers register their probe types on startup.</p>
        ) : (
          <div className="grid gap-3">
            {probeTypes?.map((pt) => (
              <div
                key={pt.id}
                className="border rounded p-3 bg-gray-50 cursor-pointer hover:bg-gray-100 transition-colors"
                onClick={() => {
                  if (watchers?.length) {
                    setEditingConfig(null);
                    setInitialProbeTypeId(pt.id);
                    setShowForm(true);
                  }
                }}
                title="Click to add a probe of this type"
              >
                <div className="flex items-center justify-between">
                  <div>
                    <span className="font-medium">{pt.name}</span>
                    <span className="text-gray-400 text-sm ml-2">v{pt.version}</span>
                  </div>
                  <span className="text-xs text-gray-400">+ Add</span>
                </div>
                <p className="text-sm text-gray-600 mt-1">{pt.description}</p>
              </div>
            ))}
          </div>
        )}
      </div>

      {/* Probe Configs Section */}
      <div className="bg-white rounded-lg shadow p-6 border border-gray-200">
        <div className="flex items-center justify-between mb-4">
          <h2 className="text-lg font-semibold">Probe Configurations</h2>
          <button
            onClick={() => { setEditingConfig(null); setInitialProbeTypeId(undefined); setShowForm(true); }}
            disabled={!probeTypes?.length || !watchers?.length}
            className="px-4 py-2 bg-green-600 text-white rounded hover:bg-green-700 disabled:opacity-50"
          >
            Add Probe
          </button>
        </div>

        {configsLoading ? (
          <p className="text-gray-500">Loading configurations...</p>
        ) : configs?.length === 0 ? (
          <p className="text-gray-500">No probes configured yet.</p>
        ) : (
          <div className="divide-y">
            {configs?.map((cfg) => (
              <div key={cfg.id} className="py-3 flex items-center justify-between">
                <div>
                  <span className="font-medium">{cfg.name}</span>
                  <span className="text-gray-400 text-sm ml-2">({cfg.probe_type_name})</span>
                  {cfg.watcher_name && (
                    <span className="text-gray-400 text-sm ml-2">@{cfg.watcher_name}</span>
                  )}
                  {!cfg.enabled && (
                    <span className="ml-2 text-xs px-2 py-0.5 rounded bg-gray-200 text-gray-600">
                      paused
                    </span>
                  )}
                  {cfg.group_path && (
                    <span className="text-xs text-gray-400 ml-2">{cfg.group_path}</span>
                  )}
                  {cfg.keywords?.length ? (
                    <span className="text-xs text-blue-400 ml-2">[{cfg.keywords.join(', ')}]</span>
                  ) : null}
                </div>
                <div className="flex gap-2">
                  <button
                    onClick={() => { setEditingConfig(cfg); setShowForm(true); }}
                    className="text-blue-600 hover:text-blue-800 text-sm"
                  >
                    Edit
                  </button>
                  <button
                    onClick={() => {
                      if (confirm('Delete this probe configuration?')) {
                        deleteMutation.mutate(cfg.id);
                      }
                    }}
                    className="text-red-600 hover:text-red-800 text-sm"
                  >
                    Delete
                  </button>
                </div>
              </div>
            ))}
          </div>
        )}
      </div>

      {/* Form Modal */}
      {showForm && watchers && (
        <ProbeConfigForm
          watchers={watchers}
          editingConfig={editingConfig}
          initialProbeTypeId={initialProbeTypeId}
          onClose={() => setShowForm(false)}
          onSaved={() => {
            setShowForm(false);
            queryClient.invalidateQueries({ queryKey: ['probeConfigs'] });
          }}
        />
      )}
    </div>
  );
}
