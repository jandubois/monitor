import { useState } from 'react';
import ReactMarkdown from 'react-markdown';
import rehypeRaw from 'rehype-raw';
import type { ProbeConfig } from '../api/types';

interface ProbeRowProps {
  config: ProbeConfig;
  isRunning?: boolean;
  onClick?: () => void;
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
  if (!dateStr) return '';
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

function PlayIcon() {
  return (
    <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M14.752 11.168l-3.197-2.132A1 1 0 0010 9.87v4.263a1 1 0 001.555.832l3.197-2.132a1 1 0 000-1.664z" />
      <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
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

export function ProbeRow({ config, isRunning, onClick, onEdit, onRerun, onPauseToggle }: ProbeRowProps) {
  const isPaused = !config.enabled;
  const [isExpanded, setIsExpanded] = useState(false);
  const hasDetails = config.last_message && config.last_message.includes('\n');
  const summary = config.last_summary || config.last_message?.split('\n')[0] || '';

  return (
    <div className={`border-b border-gray-100 ${isPaused ? 'opacity-50' : ''}`}>
      {/* Line 1: Name and timing */}
      <div className="flex items-center justify-between px-3 py-1.5">
        <div className="flex items-center gap-2 min-w-0">
          <div className={`w-2 h-2 rounded-full flex-shrink-0 ${getStatusColor(config.last_status)}`} />
          <span className="font-medium text-gray-900 truncate">{config.name}</span>
          {isPaused && (
            <span className="text-xs px-1 py-0.5 bg-gray-200 text-gray-500 rounded">paused</span>
          )}
        </div>
        <div className="text-xs text-gray-400 flex-shrink-0 ml-2">
          {formatRelativeTime(config.last_executed_at)}
          {!isPaused && config.next_run_at && (
            <span className="ml-1">| {isRunning ? 'running' : formatNextRun(config.next_run_at)}</span>
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
                disabled={isRunning}
                className={`p-1 rounded ${
                  isRunning
                    ? 'text-blue-500 cursor-not-allowed'
                    : 'text-gray-400 hover:text-gray-600 hover:bg-gray-100'
                }`}
                title={isRunning ? 'Running...' : 'Rerun'}
              >
                {isRunning ? <SpinnerIcon /> : <PlayIcon />}
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
