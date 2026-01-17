import type { ProbeConfig } from '../api/types';
import { StatusBadge } from './StatusBadge';

interface ProbeCardProps {
  config: ProbeConfig;
  onClick?: () => void;
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

export function ProbeCard({ config, onClick }: ProbeCardProps) {
  return (
    <div
      onClick={onClick}
      className="bg-white rounded-lg shadow p-4 cursor-pointer hover:shadow-md transition-shadow border border-gray-200"
    >
      <div className="flex items-start justify-between">
        <div className="flex-1 min-w-0">
          <h3 className="text-lg font-medium text-gray-900 truncate">{config.name}</h3>
          <p className="text-sm text-gray-500">{config.probe_type_name}</p>
        </div>
        <StatusBadge status={config.last_status} />
      </div>

      {config.last_message && (
        <p className="mt-2 text-sm text-gray-600 line-clamp-2">{config.last_message}</p>
      )}

      <div className="mt-3 flex items-center justify-between text-xs text-gray-400">
        <span>Every {config.interval}</span>
        <span>{formatRelativeTime(config.last_executed_at)}</span>
      </div>
    </div>
  );
}
