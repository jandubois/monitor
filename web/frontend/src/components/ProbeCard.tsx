import type { ProbeConfig } from '../api/types';
import { StatusBadge } from './StatusBadge';

interface ProbeCardProps {
  config: ProbeConfig;
  onStatusClick?: () => void;
  onEdit?: () => void;
  onRerun?: () => void;
  onPauseToggle?: () => void;
}

function formatRelativeTime(dateStr: string | undefined): string {
  if (!dateStr) return 'never';
  const date = new Date(dateStr);
  const now = new Date();
  const diffMs = now.getTime() - date.getTime();
  const diffSec = Math.floor(diffMs / 1000);
  const diffMin = Math.floor(diffSec / 60);
  const diffHour = Math.floor(diffMin / 60);
  const diffDay = Math.floor(diffHour / 24);

  if (diffSec < 60) return `${diffSec}s ago`;
  if (diffMin < 60) return `${diffMin}m ago`;
  if (diffHour < 24) return `${diffHour}h ago`;
  return `${diffDay}d ago`;
}

function formatNextRun(dateStr: string | undefined): string {
  if (!dateStr) return 'not scheduled';
  const date = new Date(dateStr);
  const now = new Date();
  const diffMs = date.getTime() - now.getTime();

  if (diffMs < 0) return 'pending';

  const diffSec = Math.floor(diffMs / 1000);
  const diffMin = Math.floor(diffSec / 60);
  const diffHour = Math.floor(diffMin / 60);

  if (diffSec < 60) return `in ${diffSec}s`;
  if (diffMin < 60) return `in ${diffMin}m`;
  if (diffHour < 24) return `in ${diffHour}h`;
  return date.toLocaleDateString();
}

export function ProbeCard({ config, onStatusClick, onEdit, onRerun, onPauseToggle }: ProbeCardProps) {
  const isPaused = !config.enabled;

  return (
    <div className={`bg-white rounded-lg shadow p-4 border ${isPaused ? 'border-gray-300 opacity-60' : 'border-gray-200'}`}>
      <div className="flex items-start justify-between">
        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-2">
            <h3 className="text-lg font-medium text-gray-900 truncate">{config.name}</h3>
            {isPaused && (
              <span className="text-xs px-1.5 py-0.5 bg-gray-200 text-gray-600 rounded">paused</span>
            )}
          </div>
          <p className="text-sm text-gray-500">{config.probe_type_name}</p>
        </div>
        <button
          onClick={onStatusClick}
          className="hover:opacity-80 transition-opacity"
          title="View details"
        >
          <StatusBadge status={config.last_status} />
        </button>
      </div>

      {config.last_message && (
        <p className="mt-2 text-sm text-gray-600 line-clamp-2">{config.last_message}</p>
      )}

      <div className="mt-3 flex items-center justify-between">
        <div className="flex gap-2">
          <button
            onClick={onEdit}
            className="text-xs px-2 py-1 text-gray-600 hover:text-gray-900 hover:bg-gray-100 rounded"
          >
            Edit
          </button>
          {isPaused ? (
            <button
              onClick={onPauseToggle}
              className="text-xs px-2 py-1 text-green-600 hover:text-green-800 hover:bg-green-50 rounded"
            >
              Resume
            </button>
          ) : (
            <>
              <button
                onClick={onRerun}
                className="text-xs px-2 py-1 text-gray-600 hover:text-gray-900 hover:bg-gray-100 rounded"
              >
                Rerun
              </button>
              <button
                onClick={onPauseToggle}
                className="text-xs px-2 py-1 text-gray-600 hover:text-gray-900 hover:bg-gray-100 rounded"
              >
                Pause
              </button>
            </>
          )}
        </div>
        <div className="text-xs text-gray-400">
          <span title={`Last: ${formatRelativeTime(config.last_executed_at)}`}>
            {isPaused ? 'paused' : formatNextRun(config.next_run_at)}
          </span>
        </div>
      </div>
    </div>
  );
}
