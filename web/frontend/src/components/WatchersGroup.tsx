import { useState, useEffect } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '../api/client';
import type { WatcherEvent } from '../api/types';

export function WatchersGroup() {
  const queryClient = useQueryClient();
  const [expandedWatcher, setExpandedWatcher] = useState<number | null>(null);

  const { data: watchers } = useQuery({
    queryKey: ['watchers'],
    queryFn: () => api.getWatchers(),
    refetchInterval: 30000,
  });

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

  // Calculate if there are actions required
  const pendingApprovals = watchers?.filter(w => !w.approved).length || 0;
  const totalUnacknowledged = unacknowledgedEvents?.length || 0;
  const hasActionsRequired = pendingApprovals > 0 || totalUnacknowledged > 0;

  // Auto-expand when actions are required
  const [isCollapsed, setIsCollapsed] = useState(!hasActionsRequired);

  useEffect(() => {
    if (hasActionsRequired) {
      setIsCollapsed(false);
    }
  }, [hasActionsRequired]);

  // Fetch events for expanded watcher
  const { data: watcherEvents } = useQuery({
    queryKey: ['watcherEvents', expandedWatcher],
    queryFn: () => expandedWatcher ? api.getWatcherEvents(expandedWatcher) : Promise.resolve([]),
    enabled: !!expandedWatcher,
  });

  // Fetch probe configs to find watcher-health probes
  const { data: probeConfigs } = useQuery({
    queryKey: ['probeConfigs'],
    queryFn: () => api.getProbeConfigs(),
  });

  // Trigger all watcher-health probes to refresh status immediately
  const triggerWatcherHealthProbes = async () => {
    const healthProbes = probeConfigs?.filter(c => c.probe_type_name === 'watcher-health') || [];
    for (const probe of healthProbes) {
      try {
        await api.triggerProbe(probe.id);
      } catch {
        // Ignore errors - probes will run on schedule anyway
      }
    }
    // Refresh queries after probes complete
    setTimeout(() => {
      queryClient.invalidateQueries({ queryKey: ['probeConfigs'] });
      queryClient.invalidateQueries({ queryKey: ['watchers'] });
      queryClient.invalidateQueries({ queryKey: ['status'] });
    }, 2000);
  };

  const pauseWatcherMutation = useMutation({
    mutationFn: ({ id, paused }: { id: number; paused: boolean }) => api.setWatcherPaused(id, paused),
    onSuccess: (_data, { paused }) => {
      queryClient.invalidateQueries({ queryKey: ['watchers'] });
      queryClient.invalidateQueries({ queryKey: ['status'] });
      if (!paused) {
        // Approval auto-acknowledges token_changed events, so refresh the events list
        queryClient.invalidateQueries({ queryKey: ['watcherEvents'] });
      }
      // Trigger watcher-health probes to update status immediately
      triggerWatcherHealthProbes();
    },
  });

  const deleteWatcherMutation = useMutation({
    mutationFn: (id: number) => api.deleteWatcher(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['watchers'] });
      queryClient.invalidateQueries({ queryKey: ['probeConfigs'] });
    },
  });

  const acknowledgeEventMutation = useMutation({
    mutationFn: (id: number) => api.acknowledgeWatcherEvent(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['watcherEvents'] });
      // Trigger watcher-health probes to update status after acknowledging events
      triggerWatcherHealthProbes();
    },
  });

  if (!watchers?.length) {
    return null;
  }

  return (
    <div className="bg-white rounded-lg shadow border border-gray-200 overflow-hidden">
      <button
        onClick={() => setIsCollapsed(!isCollapsed)}
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
        <h2 className="text-lg font-semibold text-gray-700">Watchers</h2>
        <div className="flex gap-2 text-xs">
          {pendingApprovals > 0 && (
            <span className="px-2 py-0.5 rounded bg-orange-100 text-orange-700 font-medium">
              {pendingApprovals} pending approval
            </span>
          )}
          {totalUnacknowledged > 0 && (
            <span className="px-2 py-0.5 rounded bg-red-100 text-red-700 font-medium">
              {totalUnacknowledged} event{totalUnacknowledged > 1 ? 's' : ''} to acknowledge
            </span>
          )}
          {(() => {
            const healthyCount = watchers.filter(w => w.approved && w.healthy && !w.paused).length;
            const pausedCount = watchers.filter(w => w.approved && w.paused).length;
            return (
              <>
                {healthyCount > 0 && (
                  <span className="text-green-600">{healthyCount} healthy</span>
                )}
                {pausedCount > 0 && (
                  <span className="text-gray-400">{pausedCount} paused</span>
                )}
              </>
            );
          })()}
        </div>
      </button>

      {!isCollapsed && (
        <div className="divide-y divide-gray-300">
          {watchers.map((w) => (
            <div key={w.id} className={`bg-white ${w.paused ? 'opacity-50' : ''}`}>
              <div className="px-4 py-3 flex items-center justify-between">
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
                  {w.approved && (
                    <span className={`text-xs px-2 py-0.5 rounded ${w.healthy ? 'bg-green-100 text-green-700' : 'bg-red-100 text-red-700'}`}>
                      {w.healthy ? 'healthy' : 'unhealthy'}
                    </span>
                  )}
                  {!w.approved && (
                    <span className="text-xs px-2 py-0.5 rounded bg-orange-100 text-orange-700">
                      pending approval
                    </span>
                  )}
                  {w.approved && w.paused && (
                    <span className="text-xs px-2 py-0.5 rounded bg-gray-200 text-gray-500">
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
                <div className="border-t border-gray-200 bg-gray-50 px-4 py-3 ml-6">
                  <div className="flex items-center justify-between mb-2">
                    <h4 className="text-sm font-medium text-gray-700">Recent Events</h4>
                    {watcherEvents?.some(e => !e.acknowledged) && (
                      <button
                        onClick={() => {
                          const unacked = watcherEvents?.filter(e => !e.acknowledged) || [];
                          unacked.forEach(e => acknowledgeEventMutation.mutate(e.id));
                        }}
                        className="text-xs text-blue-600 hover:text-blue-800"
                      >
                        Acknowledge All
                      </button>
                    )}
                  </div>
                  {watcherEvents?.length === 0 ? (
                    <p className="text-sm text-gray-500">No events recorded</p>
                  ) : (
                    <div className="space-y-2">
                      {watcherEvents?.slice(0, 10).map((event: WatcherEvent) => (
                        <div
                          key={event.id}
                          className={`text-sm p-2 rounded border ${
                            event.acknowledged
                              ? 'bg-white border-gray-200'
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
  );
}
