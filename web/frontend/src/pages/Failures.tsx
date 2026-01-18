import { useState, useEffect } from 'react';
import { useQuery } from '@tanstack/react-query';
import { api } from '../api/client';
import type { ProbeResult } from '../api/types';

interface FailuresProps {
  onBack: () => void;
  onProbeClick: (configId: number) => void;
}

const PAGE_SIZE = 20;

export function Failures({ onBack, onProbeClick }: FailuresProps) {
  const [page, setPage] = useState(0);

  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        onBack();
      }
    };
    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [onBack]);

  const { data: failures, isLoading } = useQuery({
    queryKey: ['failures', page],
    queryFn: () =>
      api.getResults({
        status: ['critical', 'unknown', 'warning'],
        limit: PAGE_SIZE,
        offset: page * PAGE_SIZE,
      }),
    refetchInterval: 30000,
  });

  const formatTime = (timestamp: string) => {
    const date = new Date(timestamp);
    const now = new Date();
    const diff = now.getTime() - date.getTime();

    if (diff < 60000) return 'just now';
    if (diff < 3600000) return `${Math.floor(diff / 60000)}m ago`;
    if (diff < 86400000) return `${Math.floor(diff / 3600000)}h ago`;

    return date.toLocaleDateString(undefined, {
      month: 'short',
      day: 'numeric',
      hour: '2-digit',
      minute: '2-digit',
    });
  };

  const statusColor = (status: string) => {
    switch (status) {
      case 'critical':
        return 'bg-red-100 text-red-800 border-red-200';
      case 'warning':
        return 'bg-yellow-100 text-yellow-800 border-yellow-200';
      case 'unknown':
        return 'bg-gray-100 text-gray-800 border-gray-200';
      default:
        return 'bg-gray-100 text-gray-600 border-gray-200';
    }
  };

  return (
    <div className="p-6">
      <div className="mb-6 flex items-center gap-4">
        <button
          onClick={onBack}
          className="text-gray-600 hover:text-gray-900"
        >
          ← Back
        </button>
        <h1 className="text-2xl font-bold text-gray-900">Failure Log</h1>
      </div>

      {isLoading ? (
        <div className="text-center py-12 text-gray-500">Loading failures...</div>
      ) : failures?.length === 0 ? (
        <div className="bg-white rounded-lg shadow p-8 text-center text-gray-500">
          No failures recorded.
        </div>
      ) : (
        <>
          <div className="bg-white rounded-lg shadow overflow-hidden">
            <table className="min-w-full divide-y divide-gray-200">
              <thead className="bg-gray-50">
                <tr>
                  <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                    Time
                  </th>
                  <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                    Probe
                  </th>
                  <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                    Status
                  </th>
                  <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                    Message
                  </th>
                </tr>
              </thead>
              <tbody className="bg-white divide-y divide-gray-200">
                {failures?.map((result: ProbeResult) => (
                  <tr
                    key={result.id}
                    className="hover:bg-gray-50 cursor-pointer"
                    onClick={() => onProbeClick(result.probe_config_id)}
                  >
                    <td className="px-6 py-4 whitespace-nowrap text-sm text-gray-500">
                      {formatTime(result.executed_at)}
                    </td>
                    <td className="px-6 py-4 whitespace-nowrap text-sm font-medium text-gray-900">
                      {result.config_name}
                    </td>
                    <td className="px-6 py-4 whitespace-nowrap">
                      <span
                        className={`inline-flex px-2 py-0.5 text-xs font-medium rounded border ${statusColor(
                          result.status
                        )}`}
                      >
                        {result.status}
                      </span>
                    </td>
                    <td className="px-6 py-4 text-sm text-gray-600 max-w-md truncate">
                      {result.message}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>

          <div className="mt-4 flex items-center justify-between">
            <button
              onClick={() => setPage((p) => Math.max(0, p - 1))}
              disabled={page === 0}
              className="px-4 py-2 text-sm font-medium text-gray-700 bg-white border border-gray-300 rounded hover:bg-gray-50 disabled:opacity-50 disabled:cursor-not-allowed"
            >
              Previous
            </button>
            <span className="text-sm text-gray-500">Page {page + 1}</span>
            <button
              onClick={() => setPage((p) => p + 1)}
              disabled={!failures || failures.length < PAGE_SIZE}
              className="px-4 py-2 text-sm font-medium text-gray-700 bg-white border border-gray-300 rounded hover:bg-gray-50 disabled:opacity-50 disabled:cursor-not-allowed"
            >
              Next
            </button>
          </div>
        </>
      )}
    </div>
  );
}
