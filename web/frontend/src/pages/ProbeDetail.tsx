import { useState, useEffect } from 'react';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { LineChart, Line, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer } from 'recharts';
import { api } from '../api/client';
import { StatusBadge } from '../components/StatusBadge';
import { ProbeConfigForm } from '../components/ProbeConfigForm';
import type { ProbeConfig, ProbeResult } from '../api/types';

interface ProbeDetailProps {
  config: ProbeConfig;
  onBack: () => void;
  onConfigUpdated?: (config: ProbeConfig) => void;
}

function formatDate(dateStr: string): string {
  return new Date(dateStr).toLocaleString();
}

export function ProbeDetail({ config: initialConfig, onBack, onConfigUpdated }: ProbeDetailProps) {
  const [showEditForm, setShowEditForm] = useState(false);
  const [config, setConfig] = useState(initialConfig);
  const queryClient = useQueryClient();

  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape' && !showEditForm) {
        onBack();
      }
    };
    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [onBack, showEditForm]);

  const { data: results, isLoading } = useQuery({
    queryKey: ['probeResults', config.id],
    queryFn: () => api.getProbeResults(config.id),
    refetchInterval: 30000,
  });

  const { data: watchers } = useQuery({
    queryKey: ['watchers'],
    queryFn: () => api.getWatchers(),
    enabled: showEditForm,
  });

  const { data: probeTypes } = useQuery({
    queryKey: ['probeTypes'],
    queryFn: () => api.getProbeTypes(),
    enabled: showEditForm,
  });

  const handleSaved = async () => {
    setShowEditForm(false);
    // Refresh the config data
    const updatedConfig = await api.getProbeConfig(config.id);
    setConfig(updatedConfig);
    onConfigUpdated?.(updatedConfig);
    queryClient.invalidateQueries({ queryKey: ['probeConfigs'] });
  };

  const chartData = results
    ?.slice(0, 50)
    .reverse()
    .map((r) => ({
      time: new Date(r.executed_at).toLocaleTimeString(),
      duration: r.duration_ms,
      ...r.metrics,
    }));

  const metricKeys = results?.[0]?.metrics
    ? Object.keys(results[0].metrics).filter((k) => typeof results[0].metrics![k] === 'number')
    : [];

  return (
    <div className="p-6">
      <button
        onClick={onBack}
        className="mb-4 text-blue-600 hover:text-blue-800 flex items-center gap-1"
      >
        &larr; Back to Dashboard
      </button>

      <div className="bg-white rounded-lg shadow p-6 mb-6 border border-gray-200">
        <div className="flex items-start justify-between mb-4">
          <div>
            <h1 className="text-2xl font-bold text-gray-900">{config.name}</h1>
            <p className="text-gray-500">{config.probe_type_name}</p>
          </div>
          <div className="flex items-center gap-3">
            <button
              onClick={() => setShowEditForm(true)}
              className="px-3 py-1.5 text-sm bg-gray-100 text-gray-700 rounded hover:bg-gray-200"
            >
              Edit
            </button>
            <StatusBadge status={config.last_status} size="lg" />
          </div>
        </div>

        <div className="grid grid-cols-2 md:grid-cols-4 gap-4 text-sm">
          <div>
            <span className="text-gray-500">Interval</span>
            <div className="font-medium">{config.interval}</div>
          </div>
          <div>
            <span className="text-gray-500">Timeout</span>
            <div className="font-medium">{config.timeout_seconds}s</div>
          </div>
          <div>
            <span className="text-gray-500">Enabled</span>
            <div className="font-medium">{config.enabled ? 'Yes' : 'No'}</div>
          </div>
          <div>
            <span className="text-gray-500">Last Run</span>
            <div className="font-medium">
              {config.last_executed_at ? formatDate(config.last_executed_at) : 'Never'}
            </div>
          </div>
        </div>

        {config.arguments && Object.keys(config.arguments).length > 0 && (
          <div className="mt-4 pt-4 border-t">
            <h3 className="text-sm font-medium text-gray-500 mb-2">Arguments</h3>
            <pre className="text-sm bg-gray-50 p-2 rounded overflow-auto">
              {JSON.stringify(config.arguments, null, 2)}
            </pre>
          </div>
        )}
      </div>

      {chartData && chartData.length > 0 && (
        <div className="bg-white rounded-lg shadow p-6 mb-6 border border-gray-200">
          <h2 className="text-lg font-semibold mb-4">Duration (ms)</h2>
          <div className="h-64">
            <ResponsiveContainer width="100%" height="100%">
              <LineChart data={chartData}>
                <CartesianGrid strokeDasharray="3 3" />
                <XAxis dataKey="time" tick={{ fontSize: 12 }} />
                <YAxis tick={{ fontSize: 12 }} />
                <Tooltip />
                <Line type="monotone" dataKey="duration" stroke="#3b82f6" dot={false} />
              </LineChart>
            </ResponsiveContainer>
          </div>
        </div>
      )}

      {metricKeys.length > 0 && chartData && (
        <div className="bg-white rounded-lg shadow p-6 mb-6 border border-gray-200">
          <h2 className="text-lg font-semibold mb-4">Metrics</h2>
          <div className="h-64">
            <ResponsiveContainer width="100%" height="100%">
              <LineChart data={chartData}>
                <CartesianGrid strokeDasharray="3 3" />
                <XAxis dataKey="time" tick={{ fontSize: 12 }} />
                <YAxis tick={{ fontSize: 12 }} />
                <Tooltip />
                {metricKeys.slice(0, 3).map((key, i) => (
                  <Line
                    key={key}
                    type="monotone"
                    dataKey={key}
                    stroke={['#3b82f6', '#10b981', '#f59e0b'][i]}
                    dot={false}
                  />
                ))}
              </LineChart>
            </ResponsiveContainer>
          </div>
        </div>
      )}

      <div className="bg-white rounded-lg shadow border border-gray-200">
        <h2 className="text-lg font-semibold p-4 border-b">Recent Results</h2>
        {isLoading ? (
          <div className="p-4 text-gray-500">Loading results...</div>
        ) : results?.length === 0 ? (
          <div className="p-4 text-gray-500">No results yet</div>
        ) : (
          <div className="divide-y">
            {results?.slice(0, 20).map((result: ProbeResult) => (
              <div key={result.id} className="p-4">
                <div className="flex items-center justify-between mb-2">
                  <StatusBadge status={result.status} size="sm" />
                  <span className="text-sm text-gray-500">{formatDate(result.executed_at)}</span>
                </div>
                <p className="text-sm text-gray-700">{result.message}</p>
                <div className="mt-1 text-xs text-gray-400">
                  Duration: {result.duration_ms}ms
                </div>
              </div>
            ))}
          </div>
        )}
      </div>

      {showEditForm && probeTypes && watchers && (
        <ProbeConfigForm
          probeTypes={probeTypes}
          watchers={watchers}
          editingConfig={config}
          onClose={() => setShowEditForm(false)}
          onSaved={handleSaved}
        />
      )}
    </div>
  );
}
