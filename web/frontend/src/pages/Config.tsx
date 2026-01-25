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

  const discoverMutation = useMutation({
    mutationFn: () => api.discoverProbeTypes(),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['probeTypes'] });
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

      {/* Probe Types Section */}
      <div className="bg-white rounded-lg shadow p-6 border border-gray-200">
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
