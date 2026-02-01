import { useState } from 'react';
import ReactMarkdown from 'react-markdown';
import rehypeRaw from 'rehype-raw';
import type { ProbeConfig } from '../api/types';
import { getColorByIndex } from '../utils/keywordColors';

interface ProbeRowProps {
  config: ProbeConfig;
  keywordColors?: Record<string, number>;
  secondaryLabel?: { text: string; color?: string };
  isRunning?: boolean;
  onClick?: () => void;
  onEdit?: () => void;
  onRerun?: () => void;
  onPauseToggle?: () => void;
  onDelete?: () => void;
  onKeywordClick?: (keyword: string) => void;
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

function formatNextRun(dateStr: string | undefined, timeoutSeconds: number): string {
  if (!dateStr) return '';
  const date = new Date(dateStr);
  const now = new Date();
  const diffMs = date.getTime() - now.getTime();

  if (diffMs < 0) {
    // Check if overdue (more than 3x timeout)
    const overdueThresholdMs = timeoutSeconds * 3 * 1000;
    if (-diffMs > overdueThresholdMs) return 'overdue';
    return 'pending';
  }

  const diffSec = Math.floor(diffMs / 1000);
  const diffMin = Math.floor(diffSec / 60);
  const diffHour = Math.floor(diffMin / 60);

  if (diffSec < 60) return `in ${diffSec}s`;
  if (diffMin < 60) return `in ${diffMin}m`;
  if (diffHour < 24) return `in ${diffHour}h`;
  return date.toLocaleDateString();
}

function parseInterval(interval: string): number {
  const match = interval.match(/^(\d+)(s|m|h|d)$/);
  if (!match) return 0;
  const value = parseInt(match[1], 10);
  const unit = match[2];
  switch (unit) {
    case 's': return value * 1000;
    case 'm': return value * 60 * 1000;
    case 'h': return value * 60 * 60 * 1000;
    case 'd': return value * 24 * 60 * 60 * 1000;
    default: return 0;
  }
}

// Returns how long the probe is overdue in milliseconds, or 0 if not overdue
function getOverdueMs(nextRunAt: string | undefined, timeoutSeconds: number): number {
  if (!nextRunAt) return 0;
  const date = new Date(nextRunAt);
  const now = new Date();
  const diffMs = date.getTime() - now.getTime();
  const overdueThresholdMs = timeoutSeconds * 3 * 1000;
  if (diffMs < 0 && -diffMs > overdueThresholdMs) {
    return -diffMs;
  }
  return 0;
}

function isOverdue(nextRunAt: string | undefined, timeoutSeconds: number): boolean {
  return getOverdueMs(nextRunAt, timeoutSeconds) > 0;
}

// Get effective status considering overdue state
function getEffectiveStatus(
  lastStatus: string | undefined,
  nextRunAt: string | undefined,
  timeoutSeconds: number,
  interval: string
): string | undefined {
  // If already warning or critical, keep it
  if (lastStatus === 'warning' || lastStatus === 'critical') {
    return lastStatus;
  }

  const overdueMs = getOverdueMs(nextRunAt, timeoutSeconds);
  if (overdueMs === 0) {
    return lastStatus;
  }

  // Overdue: escalate based on how long
  const intervalMs = parseInterval(interval);
  if (intervalMs > 0 && overdueMs > intervalMs) {
    // Overdue by more than one full interval - critical
    return 'critical';
  }

  // Overdue but less than one interval - warning
  return 'warning';
}

// Status indicator colors
function getStatusColor(status: string | undefined): string {
  switch (status) {
    case 'ok': return 'bg-green-500';
    case 'warning': return 'bg-yellow-500';
    case 'critical': return 'bg-red-500';
    default: return 'bg-gray-400';
  }
}

// Icon components
function ChevronIcon({ expanded }: { expanded: boolean }) {
  return (
    <svg
      className={`w-4 h-4 transition-transform ${expanded ? 'rotate-90' : ''}`}
      fill="none"
      stroke="currentColor"
      viewBox="0 0 24 24"
    >
      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 5l7 7-7 7" />
    </svg>
  );
}

function InfoIcon() {
  return (
    <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
    </svg>
  );
}

function EditIcon() {
  return (
    <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z" />
    </svg>
  );
}

function RerunIcon() {
  return (
    <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
    </svg>
  );
}

function SpinnerIcon() {
  return (
    <svg className="w-4 h-4 animate-spin" fill="none" viewBox="0 0 24 24">
      <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
      <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z" />
    </svg>
  );
}

function PauseIcon() {
  return (
    <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M10 9v6m4-6v6m7-3a9 9 0 11-18 0 9 9 0 0118 0z" />
    </svg>
  );
}

function ResumeIcon() {
  return (
    <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M14.752 11.168l-3.197-2.132A1 1 0 0010 9.87v4.263a1 1 0 001.555.832l3.197-2.132a1 1 0 000-1.664z" />
      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
    </svg>
  );
}

function TrashIcon() {
  return (
    <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
    </svg>
  );
}

export function ProbeRow({ config, keywordColors = {}, secondaryLabel, isRunning, onClick, onEdit, onRerun, onPauseToggle, onDelete, onKeywordClick }: ProbeRowProps) {
  const isPaused = !config.enabled;
  const [isExpanded, setIsExpanded] = useState(false);
  const summary = config.last_summary || config.last_message?.split('\n')[0] || '';
  const hasDetails = config.last_message && config.last_message !== summary;
  const probeIsOverdue = !isPaused && !isRunning && isOverdue(config.next_run_at, config.timeout_seconds);
  const effectiveStatus = isPaused || isRunning
    ? config.last_status
    : getEffectiveStatus(config.last_status, config.next_run_at, config.timeout_seconds, config.interval);

  return (
    <div className={`border-b border-gray-300 ${isPaused ? 'opacity-50' : ''}`}>
      {/* Line 1: Name and timing */}
      <div className="flex items-center justify-between px-3 py-1.5">
        <div className="flex items-center gap-2 min-w-0">
          <div className={`w-2 h-2 rounded-full flex-shrink-0 ${getStatusColor(effectiveStatus)}`} />
          <span className="font-medium text-gray-900 truncate">{config.name}</span>
          {config.orphaned && (
            <span className="text-xs px-1 py-0.5 bg-red-100 text-red-600 rounded" title="No watcher provides this probe type">orphaned</span>
          )}
          {probeIsOverdue && !config.orphaned && (
            <span className="text-xs px-1 py-0.5 bg-orange-100 text-orange-600 rounded" title="Probe has not run as scheduled">overdue</span>
          )}
          {isPaused && !config.orphaned && !probeIsOverdue && (
            <span className="text-xs px-1 py-0.5 bg-gray-200 text-gray-500 rounded">paused</span>
          )}
          {config.keywords?.map(kw => {
            const colorIndex = keywordColors[kw] ?? 0;
            const color = getColorByIndex(colorIndex);
            return (
              <button
                key={kw}
                onClick={(e) => { e.stopPropagation(); onKeywordClick?.(kw); }}
                className={`text-xs px-1.5 py-0.5 rounded ${color.bg} ${color.text} hover:opacity-80`}
              >
                {kw}
              </button>
            );
          })}
        </div>
        <div className="text-xs text-gray-400 flex-shrink-0 ml-2">
          {secondaryLabel && (
            <>
              <span className={secondaryLabel.color || 'text-gray-400'}>{secondaryLabel.text}</span>
              {' · '}
            </>
          )}
          {formatRelativeTime(config.last_executed_at)}
          {!isPaused && config.next_run_at && (
            <span className={`ml-1 ${probeIsOverdue ? 'text-orange-600' : ''}`}>
              | {isRunning ? 'running' : formatNextRun(config.next_run_at, config.timeout_seconds)}
            </span>
          )}
        </div>
      </div>

      {/* Line 2: Summary and actions */}
      <div className="flex items-center justify-between px-3 pb-1.5">
        <div className="flex items-center gap-1 min-w-0 flex-1">
          {hasDetails && (
            <button
              onClick={() => setIsExpanded(!isExpanded)}
              className="p-0.5 text-gray-400 hover:text-gray-600 flex-shrink-0"
              title={isExpanded ? 'Collapse' : 'Expand'}
            >
              <ChevronIcon expanded={isExpanded} />
            </button>
          )}
          <span className="text-sm text-gray-600 truncate">{summary}</span>
        </div>
        <div className="flex items-center gap-1 flex-shrink-0 ml-2">
          <button
            onClick={onClick}
            className="p-1 text-gray-400 hover:text-blue-600 hover:bg-blue-50 rounded"
            title="Details"
          >
            <InfoIcon />
          </button>
          <button
            onClick={onEdit}
            className="p-1 text-gray-400 hover:text-gray-600 hover:bg-gray-100 rounded"
            title="Edit"
          >
            <EditIcon />
          </button>
          {isPaused ? (
            <button
              onClick={onPauseToggle}
              className="p-1 text-gray-400 hover:text-green-600 hover:bg-green-50 rounded"
              title="Resume"
            >
              <ResumeIcon />
            </button>
          ) : (
            <>
              <button
                onClick={onRerun}
                disabled={isRunning || config.orphaned}
                className={`p-1 rounded ${
                  isRunning
                    ? 'text-blue-500 cursor-not-allowed'
                    : config.orphaned
                      ? 'text-gray-300 cursor-not-allowed'
                      : 'text-gray-400 hover:text-gray-600 hover:bg-gray-100'
                }`}
                title={config.orphaned ? 'Cannot run: no watcher provides this probe' : isRunning ? 'Running...' : 'Rerun'}
              >
                {isRunning ? <SpinnerIcon /> : <RerunIcon />}
              </button>
              <button
                onClick={onPauseToggle}
                className="p-1 text-gray-400 hover:text-orange-600 hover:bg-orange-50 rounded"
                title="Pause"
              >
                <PauseIcon />
              </button>
            </>
          )}
          <button
            onClick={onDelete}
            className="p-1 text-gray-400 hover:text-red-600 hover:bg-red-50 rounded"
            title="Delete"
          >
            <TrashIcon />
          </button>
        </div>
      </div>

      {/* Expanded details */}
      {isExpanded && config.last_message && (
        <div className="px-3 pb-2 ml-6">
          <div className="text-sm text-gray-700 prose prose-sm max-w-none bg-gray-50 rounded p-2">
            <ReactMarkdown
              rehypePlugins={[rehypeRaw]}
              components={{
                a: ({ href, children }) => (
                  <a href={href} target="_blank" rel="noopener noreferrer">{children}</a>
                ),
              }}
            >
              {config.last_message}
            </ReactMarkdown>
          </div>
        </div>
      )}
    </div>
  );
}
