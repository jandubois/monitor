import { useState, useEffect } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '../api/client';
import { ProbeConfigForm } from '../components/ProbeConfigForm';
import type { ProbeConfig } from '../api/types';

interface ConfigProps {
  onBack: () => void;
}

export function Config({ onBack }: ConfigProps) {
  const queryClient = useQueryClient();
  const [showForm, setShowForm] = useState(false);
  const [editingConfig, setEditingConfig] = useState<ProbeConfig | null>(null);
  const [initialProbeTypeId, setInitialProbeTypeId] = useState<number | undefined>();

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
              <div key={w.id} className="border rounded p-3 bg-gray-50 flex items-center justify-between">
                <div>
                  <span className="font-medium">{w.name}</span>
                  {w.version && <span className="text-gray-400 text-sm ml-2">v{w.version}</span>}
                  <span className={`ml-2 text-xs px-2 py-0.5 rounded ${w.healthy ? 'bg-green-100 text-green-700' : 'bg-red-100 text-red-700'}`}>
                    {w.healthy ? 'healthy' : 'unhealthy'}
                  </span>
                </div>
                <div className="text-sm text-gray-500">
                  {w.probe_type_count} types, {w.config_count} configs
                </div>
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
      {showForm && probeTypes && watchers && (
        <ProbeConfigForm
          probeTypes={probeTypes}
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
