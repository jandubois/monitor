import { useState, useEffect } from 'react';
import { api } from '../api/client';
import type { ProbeType, ProbeConfig, Watcher } from '../api/types';

interface ProbeConfigFormProps {
  probeTypes: ProbeType[];
  watchers: Watcher[];
  editingConfig: ProbeConfig | null;
  initialProbeTypeId?: number;
  onClose: () => void;
  onSaved: () => void;
}

export function ProbeConfigForm({ probeTypes, watchers, editingConfig, initialProbeTypeId, onClose, onSaved }: ProbeConfigFormProps) {
  const [name, setName] = useState(editingConfig?.name ?? '');
  const [probeTypeId, setProbeTypeId] = useState(editingConfig?.probe_type_id ?? initialProbeTypeId ?? probeTypes[0]?.id ?? 0);
  const [watcherId, setWatcherId] = useState<number | undefined>(editingConfig?.watcher_id ?? watchers[0]?.id);
  const [enabled, setEnabled] = useState(editingConfig?.enabled ?? true);
  const [interval, setInterval] = useState(editingConfig?.interval ?? '5m');
  const [timeout, setTimeout] = useState(editingConfig?.timeout_seconds ?? 60);
  const [groupPath, setGroupPath] = useState(editingConfig?.group_path ?? '');
  const [keywords, setKeywords] = useState(editingConfig?.keywords?.join(', ') ?? '');
  const [args, setArgs] = useState<Record<string, string>>(
    Object.fromEntries(
      Object.entries(editingConfig?.arguments ?? {}).map(([k, v]) => [k, String(v)])
    )
  );
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');

  const selectedType = probeTypes.find((pt) => pt.id === probeTypeId);

  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        onClose();
      }
    };
    document.addEventListener('keydown', handleKeyDown);
    return () => document.removeEventListener('keydown', handleKeyDown);
  }, [onClose]);

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

    // Parse keywords
    const keywordsList = keywords
      .split(',')
      .map((k) => k.trim())
      .filter((k) => k.length > 0);

    try {
      if (editingConfig) {
        await api.updateProbeConfig(editingConfig.id, {
          watcher_id: watcherId,
          name,
          enabled,
          arguments: typedArgs,
          interval,
          timeout_seconds: timeout,
          notification_channels: editingConfig.notification_channels,
          group_path: groupPath || undefined,
          keywords: keywordsList.length > 0 ? keywordsList : undefined,
        });
        // Trigger immediate rerun after edit (only if enabled)
        if (enabled) {
          await api.triggerProbe(editingConfig.id);
        }
      } else {
        const result = await api.createProbeConfig({
          probe_type_id: probeTypeId,
          watcher_id: watcherId,
          name,
          enabled,
          arguments: typedArgs,
          interval,
          timeout_seconds: timeout,
          notification_channels: [],
          group_path: groupPath || undefined,
          keywords: keywordsList.length > 0 ? keywordsList : undefined,
        });
        // Trigger immediate run for new probe
        await api.triggerProbe(result.id);
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
      <div className="bg-white rounded-lg shadow-xl w-full max-w-2xl max-h-[90vh] overflow-y-auto">
        <div className="p-6">
          <h3 className="text-lg font-semibold mb-4">
            {editingConfig ? 'Edit Probe' : 'Add Probe'}
          </h3>

          <form onSubmit={handleSubmit} className="space-y-4">
            {/* Two-column layout for main fields */}
            <div className="grid grid-cols-2 gap-4">
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

              <div>
                <label className="block text-sm font-medium text-gray-700 mb-1">Watcher</label>
                <select
                  value={watcherId}
                  onChange={(e) => setWatcherId(Number(e.target.value))}
                  className="w-full px-3 py-2 border rounded focus:ring-2 focus:ring-blue-500"
                >
                  {watchers.map((w) => (
                    <option key={w.id} value={w.id}>{w.name}</option>
                  ))}
                </select>
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
                      <option key={pt.id} value={pt.id}>{pt.name} (v{pt.version})</option>
                    ))}
                  </select>
                </div>
              )}

              <div>
                <label className="block text-sm font-medium text-gray-700 mb-1">Group</label>
                <input
                  type="text"
                  value={groupPath}
                  onChange={(e) => setGroupPath(e.target.value)}
                  className="w-full px-3 py-2 border rounded focus:ring-2 focus:ring-blue-500"
                  placeholder="e.g., Backups/Photos"
                />
              </div>

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

              <div className="col-span-2">
                <label className="block text-sm font-medium text-gray-700 mb-1">Keywords</label>
                <input
                  type="text"
                  value={keywords}
                  onChange={(e) => setKeywords(e.target.value)}
                  className="w-full px-3 py-2 border rounded focus:ring-2 focus:ring-blue-500"
                  placeholder="e.g., personal, critical, nas (comma-separated)"
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

            {/* Arguments in two columns */}
            {selectedType?.arguments && (
              <div className="border-t pt-4">
                <h4 className="text-sm font-medium text-gray-700 mb-3">Arguments</h4>
                <div className="grid grid-cols-2 gap-3">
                  {selectedType.arguments.required && Object.entries(selectedType.arguments.required).map(([key, spec]) => (
                    <div key={key}>
                      <label className="block text-sm text-gray-600 mb-1">
                        {key} <span className="text-red-500">*</span>
                        <span className="text-gray-400 ml-1">({spec.type})</span>
                      </label>
                      {spec.enum ? (
                        <select
                          value={args[key] ?? ''}
                          onChange={(e) => setArgs({ ...args, [key]: e.target.value })}
                          required
                          className="w-full px-3 py-2 border rounded focus:ring-2 focus:ring-blue-500 text-sm"
                        >
                          <option value="">Select {key}...</option>
                          {spec.enum.map((opt) => (
                            <option key={opt} value={opt}>{opt}</option>
                          ))}
                        </select>
                      ) : (
                        <input
                          type={spec.type === 'number' ? 'number' : 'text'}
                          value={args[key] ?? ''}
                          onChange={(e) => setArgs({ ...args, [key]: e.target.value })}
                          required
                          placeholder={spec.description}
                          className="w-full px-3 py-2 border rounded focus:ring-2 focus:ring-blue-500 text-sm"
                        />
                      )}
                    </div>
                  ))}
                  {selectedType.arguments.optional && Object.entries(selectedType.arguments.optional).map(([key, spec]) => (
                    <div key={key}>
                      <label className="block text-sm text-gray-600 mb-1">
                        {key}
                        <span className="text-gray-400 ml-1">({spec.type})</span>
                        {spec.default !== undefined && (
                          <span className="text-gray-400 ml-1">= {String(spec.default)}</span>
                        )}
                      </label>
                      {spec.enum ? (
                        <select
                          value={args[key] ?? ''}
                          onChange={(e) => setArgs({ ...args, [key]: e.target.value })}
                          className="w-full px-3 py-2 border rounded focus:ring-2 focus:ring-blue-500 text-sm"
                        >
                          <option value="">{spec.default ? `Default: ${spec.default}` : `Select ${key}...`}</option>
                          {spec.enum.map((opt) => (
                            <option key={opt} value={opt}>{opt}</option>
                          ))}
                        </select>
                      ) : (
                        <input
                          type={spec.type === 'number' ? 'number' : 'text'}
                          value={args[key] ?? ''}
                          onChange={(e) => setArgs({ ...args, [key]: e.target.value })}
                          placeholder={spec.description}
                          className="w-full px-3 py-2 border rounded focus:ring-2 focus:ring-blue-500 text-sm"
                        />
                      )}
                    </div>
                  ))}
                </div>
              </div>
            )}

            {error && <p className="text-red-600 text-sm">{error}</p>}

            <div className="flex justify-end gap-3 pt-4 border-t">
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
