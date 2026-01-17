import type { ProbeStatus } from '../api/types';

interface StatusBadgeProps {
  status: ProbeStatus | undefined;
  size?: 'sm' | 'md' | 'lg';
}

const statusColors: Record<ProbeStatus, string> = {
  ok: 'bg-green-500',
  warning: 'bg-yellow-500',
  critical: 'bg-red-500',
  unknown: 'bg-gray-500',
};

const sizeClasses = {
  sm: 'px-2 py-0.5 text-xs',
  md: 'px-2.5 py-1 text-sm',
  lg: 'px-3 py-1.5 text-base',
};

export function StatusBadge({ status, size = 'md' }: StatusBadgeProps) {
  if (!status) {
    return (
      <span className={`${sizeClasses[size]} rounded-full bg-gray-300 text-gray-700 font-medium`}>
        pending
      </span>
    );
  }

  return (
    <span className={`${sizeClasses[size]} rounded-full ${statusColors[status]} text-white font-medium`}>
      {status}
    </span>
  );
}
