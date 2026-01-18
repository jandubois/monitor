import { useState } from 'react';
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '../api/client';
import type { ProbeType, ProbeConfig } from '../api/types';

interface ConfigProps {
  onBack: () => void;
}

export function Config({ onBack }: ConfigProps) {
  const queryClient = useQueryClient();
  const [showForm, setShowForm] = useState(false);
  const [editingConfig, setEditingConfig] = useState<ProbeConfig | null>(null);

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

      {/* Probe Types Section */}
      <div className="bg-white rounded-lg shadow p-6 mb-6 border border-gray-200">
        <div className="flex items-center justify-between mb-4">
          <h2 className="text-lg font-semibold">Probe Types</h2>
          <button
            onClick={() => discoverMutation.mutate()}
            disabled={discoverMutation.isPending}
            className="px-4 py-2 bg-blue-600 text-white rounded hover:bg-blue-700 disabled:opacity-50"
          >
            {discoverMutation.isPending ? 'Discovering...' : 'Discover Probes'}
          </button>
        </div>

        {typesLoading ? (
          <p className="text-gray-500">Loading probe types...</p>
        ) : probeTypes?.length === 0 ? (
          <p className="text-gray-500">No probe types registered. Click "Discover Probes" to scan for available probes.</p>
        ) : (
          <div className="grid gap-3">
            {probeTypes?.map((pt) => (
              <div key={pt.id} className="border rounded p-3 bg-gray-50">
                <div className="flex items-center justify-between">
                  <div>
                    <span className="font-medium">{pt.name}</span>
                    <span className="text-gray-400 text-sm ml-2">v{pt.version}</span>
                  </div>
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
            onClick={() => { setEditingConfig(null); setShowForm(true); }}
            disabled={!probeTypes?.length}
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
                  <span className={`ml-2 text-xs px-2 py-0.5 rounded ${cfg.enabled ? 'bg-green-100 text-green-700' : 'bg-gray-100 text-gray-500'}`}>
                    {cfg.enabled ? 'enabled' : 'disabled'}
                  </span>
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
      {showForm && probeTypes && (
        <ProbeConfigForm
          probeTypes={probeTypes}
          editingConfig={editingConfig}
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

interface ProbeConfigFormProps {
  probeTypes: ProbeType[];
  editingConfig: ProbeConfig | null;
  onClose: () => void;
  onSaved: () => void;
}

function ProbeConfigForm({ probeTypes, editingConfig, onClose, onSaved }: ProbeConfigFormProps) {
  const [name, setName] = useState(editingConfig?.name ?? '');
  const [probeTypeId, setProbeTypeId] = useState(editingConfig?.probe_type_id ?? probeTypes[0]?.id ?? 0);
  const [enabled, setEnabled] = useState(editingConfig?.enabled ?? true);
  const [interval, setInterval] = useState(editingConfig?.interval ?? '5m');
  const [timeout, setTimeout] = useState(editingConfig?.timeout_seconds ?? 60);
  const [args, setArgs] = useState<Record<string, string>>(
    Object.fromEntries(
      Object.entries(editingConfig?.arguments ?? {}).map(([k, v]) => [k, String(v)])
    )
  );
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');

  const selectedType = probeTypes.find((pt) => pt.id === probeTypeId);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError('');
    setSaving(true);

    // Convert string args to appropriate types
    const typedArgs: Record<string, unknown> = {};
    if (selectedType?.arguments) {
      const allArgs = { ...selectedType.arguments.required, ...selectedType.arguments.optional };
      for (const [key, value] of Object.entries(args)) {
        if (value === '') continue;
        const spec = allArgs?.[key];
        if (spec?.type === 'number') {
          typedArgs[key] = parseFloat(value);
        } else if (spec?.type === 'boolean') {
          typedArgs[key] = value === 'true';
        } else {
          typedArgs[key] = value;
        }
      }
    }

    try {
      if (editingConfig) {
        await api.updateProbeConfig(editingConfig.id, {
          name,
          enabled,
          arguments: typedArgs,
          interval,
          timeout_seconds: timeout,
          notification_channels: editingConfig.notification_channels,
        });
      } else {
        await api.createProbeConfig({
          probe_type_id: probeTypeId,
          name,
          enabled,
          arguments: typedArgs,
          interval,
          timeout_seconds: timeout,
          notification_channels: [],
        });
      }
      onSaved();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to save');
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="fixed inset-0 bg-black/50 flex items-center justify-center p-4 z-50">
      <div className="bg-white rounded-lg shadow-xl w-full max-w-lg max-h-[90vh] overflow-y-auto">
        <div className="p-6">
          <h3 className="text-lg font-semibold mb-4">
            {editingConfig ? 'Edit Probe' : 'Add Probe'}
          </h3>

          <form onSubmit={handleSubmit} className="space-y-4">
            <div>
              <label className="block text-sm font-medium text-gray-700 mb-1">Name</label>
              <input
                type="text"
                value={name}
                onChange={(e) => setName(e.target.value)}
                required
                className="w-full px-3 py-2 border rounded focus:ring-2 focus:ring-blue-500 focus:border-blue-500"
                placeholder="My probe"
              />
            </div>

            {!editingConfig && (
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-1">Probe Type</label>
                <select
                  value={probeTypeId}
                  onChange={(e) => {
                    setProbeTypeId(Number(e.target.value));
                    setArgs({});
                  }}
                  className="w-full px-3 py-2 border rounded focus:ring-2 focus:ring-blue-500"
                >
                  {probeTypes.map((pt) => (
                    <option key={pt.id} value={pt.id}>{pt.name}</option>
                  ))}
                </select>
              </div>
            )}

            <div className="grid grid-cols-2 gap-4">
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-1">Interval</label>
                <select
                  value={interval}
                  onChange={(e) => setInterval(e.target.value)}
                  className="w-full px-3 py-2 border rounded focus:ring-2 focus:ring-blue-500"
                >
                  <option value="1m">1 minute</option>
                  <option value="5m">5 minutes</option>
                  <option value="15m">15 minutes</option>
                  <option value="30m">30 minutes</option>
                  <option value="1h">1 hour</option>
                  <option value="6h">6 hours</option>
                  <option value="1d">1 day</option>
                </select>
              </div>
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-1">Timeout (s)</label>
                <input
                  type="number"
                  value={timeout}
                  onChange={(e) => setTimeout(Number(e.target.value))}
                  min={1}
                  max={600}
                  className="w-full px-3 py-2 border rounded focus:ring-2 focus:ring-blue-500"
                />
              </div>
            </div>

            <div className="flex items-center gap-2">
              <input
                type="checkbox"
                id="enabled"
                checked={enabled}
                onChange={(e) => setEnabled(e.target.checked)}
                className="rounded"
              />
              <label htmlFor="enabled" className="text-sm text-gray-700">Enabled</label>
            </div>

            {/* Arguments */}
            {selectedType?.arguments && (
              <div className="border-t pt-4">
                <h4 className="text-sm font-medium text-gray-700 mb-3">Arguments</h4>
                <div className="space-y-3">
                  {selectedType.arguments.required && Object.entries(selectedType.arguments.required).map(([key, spec]) => (
                    <div key={key}>
                      <label className="block text-sm text-gray-600 mb-1">
                        {key} <span className="text-red-500">*</span>
                        <span className="text-gray-400 ml-1">({spec.type})</span>
                      </label>
                      <input
                        type={spec.type === 'number' ? 'number' : 'text'}
                        value={args[key] ?? ''}
                        onChange={(e) => setArgs({ ...args, [key]: e.target.value })}
                        required
                        placeholder={spec.description}
                        className="w-full px-3 py-2 border rounded focus:ring-2 focus:ring-blue-500 text-sm"
                      />
                    </div>
                  ))}
                  {selectedType.arguments.optional && Object.entries(selectedType.arguments.optional).map(([key, spec]) => (
                    <div key={key}>
                      <label className="block text-sm text-gray-600 mb-1">
                        {key}
                        <span className="text-gray-400 ml-1">({spec.type})</span>
                        {spec.default !== undefined && (
                          <span className="text-gray-400 ml-1">default: {String(spec.default)}</span>
                        )}
                      </label>
                      <input
                        type={spec.type === 'number' ? 'number' : 'text'}
                        value={args[key] ?? ''}
                        onChange={(e) => setArgs({ ...args, [key]: e.target.value })}
                        placeholder={spec.description}
                        className="w-full px-3 py-2 border rounded focus:ring-2 focus:ring-blue-500 text-sm"
                      />
                    </div>
                  ))}
                </div>
              </div>
            )}

            {error && <p className="text-red-600 text-sm">{error}</p>}

            <div className="flex justify-end gap-3 pt-4">
              <button
                type="button"
                onClick={onClose}
                className="px-4 py-2 text-gray-600 hover:text-gray-800"
              >
                Cancel
              </button>
              <button
                type="submit"
                disabled={saving}
                className="px-4 py-2 bg-blue-600 text-white rounded hover:bg-blue-700 disabled:opacity-50"
              >
                {saving ? 'Saving...' : 'Save'}
              </button>
            </div>
          </form>
        </div>
      </div>
    </div>
  );
}
