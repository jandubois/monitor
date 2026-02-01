import { useState, useEffect, useCallback, useMemo } from 'react';
import { useQuery } from '@tanstack/react-query';
import { api } from '../api/client';
import { getColorByIndex, getNextColorIndex } from '../utils/keywordColors';
import type { ProbeConfig, Watcher, ProbeType } from '../api/types';

// Expand a template string with the given context
// Supports {{key}} syntax for both probe arguments and builtins (Watcher, ProbeName, ProbeType)
// Empty values leave the placeholder unexpanded so users see what's missing
function expandTemplate(template: string, args: Record<string, string>, watcher?: Watcher, probeType?: ProbeType): string {
  return template.replace(/\{\{(\w+)\}\}/g, (match, key) => {
    // Builtins use PascalCase
    if (key === 'Watcher') return watcher?.name || match;
    if (key === 'ProbeName') return probeType?.name || match;
    if (key === 'ProbeType') return probeType?.name || match;
    // Probe arguments use the provided args; keep placeholder if empty
    return args[key] || match;
  });
}

interface ProbeConfigFormProps {
  watchers: Watcher[];
  editingConfig: ProbeConfig | null;
  initialProbeTypeId?: number;
  keywordColors?: Record<string, number>;
  onClose: () => void;
  onSaved: () => void;
  onRerun?: (id: number) => void;
}

export function ProbeConfigForm({ watchers, editingConfig, initialProbeTypeId, keywordColors = {}, onClose, onSaved, onRerun }: ProbeConfigFormProps) {
  // Fetch all probe types (not filtered by watcher)
  const { data: allProbeTypes = [] } = useQuery({
    queryKey: ['probeTypes'],
    queryFn: () => api.getProbeTypes(),
  });

  // Fetch probe configs to extract existing groups and keywords
  const { data: probeConfigs = [] } = useQuery({
    queryKey: ['probeConfigs'],
    queryFn: () => api.getProbeConfigs(),
    staleTime: 0, // Always fetch fresh data when form opens
  });

  // Extract unique group names from existing configs
  const existingGroups = useMemo(() => {
    const groups = new Set<string>();
    probeConfigs.forEach(config => {
      if (config.group_path) {
        groups.add(config.group_path);
      }
    });
    return Array.from(groups).sort();
  }, [probeConfigs]);

  // Extract unique keywords from existing configs
  const existingKeywords = useMemo(() => {
    const kw = new Set<string>();
    probeConfigs.forEach(config => {
      config.keywords?.forEach(k => kw.add(k));
    });
    return Array.from(kw).sort();
  }, [probeConfigs]);

  const [probeTypeId, setProbeTypeId] = useState<number>(editingConfig?.probe_type_id ?? initialProbeTypeId ?? 0);
  const [name, setName] = useState(editingConfig?.name ?? '');
  const [nameManuallyEdited, setNameManuallyEdited] = useState(!!editingConfig);
  const [watcherId, setWatcherId] = useState<number | undefined>(editingConfig?.watcher_id);
  const [enabled, setEnabled] = useState(editingConfig?.enabled ?? true);
  const [interval, setInterval] = useState(editingConfig?.interval ?? '5m');
  const [timeout, setTimeout] = useState(editingConfig?.timeout_seconds ?? 60);
  const [groupPath, setGroupPath] = useState(editingConfig?.group_path ?? '');
  const [isCustomGroup, setIsCustomGroup] = useState(false);
  const [selectedKeywords, setSelectedKeywords] = useState<string[]>(editingConfig?.keywords ?? []);
  const [newKeyword, setNewKeyword] = useState('');
  // Track temporary colors for new keywords that don't have a color yet
  const [tempKeywordColors, setTempKeywordColors] = useState<Record<string, number>>({});
  const [args, setArgs] = useState<Record<string, string>>(
    Object.fromEntries(
      Object.entries(editingConfig?.arguments ?? {}).map(([k, v]) => [k, String(v)])
    )
  );
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');

  const selectedType = allProbeTypes.find((pt) => pt.id === probeTypeId);

  // Get color for a keyword, assigning a temp color if it doesn't have one
  const getKeywordColor = useCallback((kw: string) => {
    // Use assigned color if available
    if (keywordColors[kw] !== undefined) {
      return getColorByIndex(keywordColors[kw]);
    }
    // Use temp color if already assigned
    if (tempKeywordColors[kw] !== undefined) {
      return getColorByIndex(tempKeywordColors[kw]);
    }
    // Assign a new temp color
    const usedIndices = [
      ...Object.values(keywordColors),
      ...Object.values(tempKeywordColors),
    ];
    const nextIndex = getNextColorIndex(usedIndices);
    setTempKeywordColors(prev => ({ ...prev, [kw]: nextIndex }));
    return getColorByIndex(nextIndex);
  }, [keywordColors, tempKeywordColors]);

  // Filter watchers to only those that support the selected probe type
  const availableWatchers = useMemo(() => {
    if (!selectedType?.watcher_ids) return [];
    return watchers.filter(w => selectedType.watcher_ids!.includes(w.id));
  }, [watchers, selectedType]);

  // Set initial probe type when data loads
  useEffect(() => {
    if (!editingConfig && allProbeTypes.length > 0 && probeTypeId === 0) {
      const initial = initialProbeTypeId && allProbeTypes.find(pt => pt.id === initialProbeTypeId);
      setProbeTypeId(initial ? initial.id : allProbeTypes[0].id);
    }
  }, [editingConfig, allProbeTypes, probeTypeId, initialProbeTypeId]);

  // When probe type changes, reset watcher to first available
  useEffect(() => {
    if (!editingConfig && availableWatchers.length > 0) {
      if (!watcherId || !availableWatchers.find(w => w.id === watcherId)) {
        setWatcherId(availableWatchers[0].id);
      }
    }
  }, [editingConfig, availableWatchers, watcherId]);

  // When probe type changes (for new probes), use the default_interval if available
  useEffect(() => {
    if (!editingConfig && selectedType?.default_interval) {
      setInterval(selectedType.default_interval);
    }
  }, [editingConfig, selectedType]);

  const selectedWatcher = watchers.find(w => w.id === watcherId);

  // Generate default name from template
  const generateDefaultName = useCallback(() => {
    if (!selectedType?.default_name) return '';
    return expandTemplate(selectedType.default_name, args, selectedWatcher, selectedType);
  }, [selectedType, args, selectedWatcher]);

  // Auto-populate name when template inputs change (unless manually edited)
  useEffect(() => {
    if (nameManuallyEdited) return;
    const defaultName = generateDefaultName();
    if (defaultName) {
      setName(defaultName);
    }
  }, [nameManuallyEdited, generateDefaultName]);

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
    const allArgs = selectedType?.arguments
      ? { ...selectedType.arguments.required, ...selectedType.arguments.optional }
      : {};

    for (const [key, value] of Object.entries(args)) {
      if (value === '') continue;
      const spec = allArgs[key];
      if (spec?.type === 'number') {
        typedArgs[key] = parseFloat(value);
      } else if (spec?.type === 'boolean') {
        typedArgs[key] = value === 'true';
      } else {
        typedArgs[key] = value;
      }
    }

    // When editing, preserve any original args that weren't in the form
    // (in case the probe type schema changed or wasn't loaded)
    if (editingConfig?.arguments) {
      for (const [key, value] of Object.entries(editingConfig.arguments)) {
        if (!(key in typedArgs) && !(key in args)) {
          typedArgs[key] = value;
        }
      }
    }

    // Use selected keywords directly
    const keywordsList = selectedKeywords;

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
          onRerun?.(editingConfig.id);
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
        onRerun?.(result.id);
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
              {/* Probe Type - first, and disabled when editing */}
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-1">Probe Type</label>
                {editingConfig ? (
                  <div className="px-3 py-2 border rounded bg-gray-50 text-gray-700">
                    {editingConfig.probe_type_name}
                  </div>
                ) : (
                  <select
                    value={probeTypeId}
                    onChange={(e) => {
                      setProbeTypeId(Number(e.target.value));
                      setArgs({});
                    }}
                    className="w-full px-3 py-2 border rounded focus:ring-2 focus:ring-blue-500"
                  >
                    {allProbeTypes.map((pt) => (
                      <option key={pt.id} value={pt.id}>{pt.name} (v{pt.version})</option>
                    ))}
                  </select>
                )}
              </div>

              {/* Watcher - filtered by probe type */}
              <div>
                <label className="block text-sm font-medium text-gray-700 mb-1">Watcher</label>
                {editingConfig ? (
                  <div className="px-3 py-2 border rounded bg-gray-50 text-gray-700">
                    {editingConfig.watcher_name}
                  </div>
                ) : (
                  <select
                    value={watcherId}
                    onChange={(e) => setWatcherId(Number(e.target.value))}
                    className="w-full px-3 py-2 border rounded focus:ring-2 focus:ring-blue-500"
                    disabled={availableWatchers.length === 0}
                  >
                    {availableWatchers.length === 0 ? (
                      <option value="">No watchers available</option>
                    ) : (
                      availableWatchers.map((w) => (
                        <option key={w.id} value={w.id}>{w.name}</option>
                      ))
                    )}
                  </select>
                )}
              </div>

              <div>
                <label className="block text-sm font-medium text-gray-700 mb-1">Name</label>
                <input
                  type="text"
                  value={name}
                  onChange={(e) => {
                    const newValue = e.target.value;
                    setName(newValue);
                    // If cleared, restore template behavior; otherwise mark as manually edited
                    setNameManuallyEdited(newValue !== '');
                  }}
                  required
                  className="w-full px-3 py-2 border rounded focus:ring-2 focus:ring-blue-500 focus:border-blue-500"
                  placeholder="My probe"
                />
              </div>

              <div>
                <label className="block text-sm font-medium text-gray-700 mb-1">Group</label>
                {isCustomGroup ? (
                  <div className="flex gap-2">
                    <input
                      type="text"
                      value={groupPath}
                      onChange={(e) => setGroupPath(e.target.value)}
                      onKeyDown={(e) => {
                        if (e.key === 'Enter') {
                          e.preventDefault();
                          if (groupPath) {
                            setIsCustomGroup(false);
                          }
                        } else if (e.key === 'Escape') {
                          setIsCustomGroup(false);
                          setGroupPath('');
                        }
                      }}
                      className="flex-1 px-3 py-2 border rounded focus:ring-2 focus:ring-blue-500"
                      placeholder="New group name"
                      autoFocus
                    />
                    <button
                      type="button"
                      onClick={() => {
                        setIsCustomGroup(false);
                        setGroupPath('');
                      }}
                      className="px-3 py-2 text-gray-500 hover:text-gray-700 border rounded"
                    >
                      Cancel
                    </button>
                  </div>
                ) : (
                  <select
                    value={!groupPath ? '__none__' : groupPath}
                    onChange={(e) => {
                      if (e.target.value === '__custom__') {
                        setIsCustomGroup(true);
                        setGroupPath('');
                      } else if (e.target.value === '__none__') {
                        setGroupPath('');
                      } else {
                        setGroupPath(e.target.value);
                      }
                    }}
                    className="w-full px-3 py-2 border rounded focus:ring-2 focus:ring-blue-500"
                  >
                    <option value="__none__">No group</option>
                    {existingGroups.map((group) => (
                      <option key={group} value={group}>{group}</option>
                    ))}
                    {groupPath && !existingGroups.includes(groupPath) && (
                      <option key={groupPath} value={groupPath}>{groupPath}</option>
                    )}
                    <option value="__custom__">+ Add new group...</option>
                  </select>
                )}
              </div>

              <div>
                <label className="block text-sm font-medium text-gray-700 mb-1">
                  Interval
                  {selectedType?.default_interval && (
                    <span className="text-gray-400 font-normal ml-1">(default: {selectedType.default_interval})</span>
                  )}
                </label>
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
                <div className="space-y-2">
                  {/* Selected keywords as removable tags */}
                  {selectedKeywords.length > 0 && (
                    <div className="flex flex-wrap gap-1">
                      {selectedKeywords.map(kw => {
                        const color = getKeywordColor(kw);
                        return (
                          <span
                            key={kw}
                            className={`inline-flex items-center gap-1 px-2 py-1 rounded text-sm ${color.bg} ${color.text}`}
                          >
                            {kw}
                            <button
                              type="button"
                              onClick={() => setSelectedKeywords(prev => prev.filter(k => k !== kw))}
                              className="opacity-60 hover:opacity-100"
                            >
                              ×
                            </button>
                          </span>
                        );
                      })}
                    </div>
                  )}
                  {/* Available keywords to add */}
                  {existingKeywords.filter(kw => !selectedKeywords.includes(kw)).length > 0 && (
                    <div className="flex flex-wrap gap-1">
                      {existingKeywords.filter(kw => !selectedKeywords.includes(kw)).map(kw => {
                        const color = getKeywordColor(kw);
                        return (
                          <button
                            key={kw}
                            type="button"
                            onClick={() => setSelectedKeywords(prev => [...prev, kw])}
                            className={`px-2 py-1 rounded text-sm opacity-60 hover:opacity-100 ${color.bg} ${color.text}`}
                          >
                            + {kw}
                          </button>
                        );
                      })}
                    </div>
                  )}
                  {/* Input for new keyword */}
                  <input
                    type="text"
                    value={newKeyword}
                    onChange={(e) => setNewKeyword(e.target.value)}
                    onKeyDown={(e) => {
                      if (e.key === 'Enter') {
                        e.preventDefault();
                        const kw = newKeyword.trim();
                        if (kw && !selectedKeywords.includes(kw)) {
                          setSelectedKeywords(prev => [...prev, kw]);
                          setNewKeyword('');
                        }
                      }
                    }}
                    className="w-full px-3 py-2 border rounded focus:ring-2 focus:ring-blue-500 text-sm"
                    placeholder="Add new keyword..."
                  />
                </div>
              </div>
            </div>

            <div className="flex items-center gap-2">
              <input
                type="checkbox"
                id="paused"
                checked={!enabled}
                onChange={(e) => setEnabled(!e.target.checked)}
                className="rounded"
              />
              <label htmlFor="paused" className="text-sm text-gray-700">Paused</label>
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
                      ) : spec.type === 'boolean' ? (
                        <div className="flex items-center gap-2 py-2">
                          <input
                            type="checkbox"
                            id={`arg-${key}`}
                            checked={args[key] === 'true'}
                            onChange={(e) => setArgs({ ...args, [key]: e.target.checked ? 'true' : 'false' })}
                            className="rounded"
                          />
                          <label htmlFor={`arg-${key}`} className="text-sm text-gray-600">{spec.description}</label>
                        </div>
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
                      ) : spec.type === 'boolean' ? (
                        <div className="flex items-center gap-2 py-2">
                          <input
                            type="checkbox"
                            id={`arg-${key}`}
                            checked={args[key] === 'true'}
                            onChange={(e) => setArgs({ ...args, [key]: e.target.checked ? 'true' : 'false' })}
                            className="rounded"
                          />
                          <label htmlFor={`arg-${key}`} className="text-sm text-gray-600">{spec.description}</label>
                        </div>
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
                disabled={saving || availableWatchers.length === 0}
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
