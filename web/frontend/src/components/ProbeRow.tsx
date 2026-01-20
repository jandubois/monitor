import { useState } from 'react';
import ReactMarkdown from 'react-markdown';
import rehypeRaw from 'rehype-raw';
import type { ProbeConfig } from '../api/types';
import { StatusBadge } from './StatusBadge';

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

export function ProbeRow({ config, isRunning, onClick, onEdit, onRerun, onPauseToggle }: ProbeRowProps) {
  const isPaused = !config.enabled;
  const [isExpanded, setIsExpanded] = useState(config.last_status !== 'ok');

  return (
    <div className={`bg-white p-4 ${isPaused ? 'opacity-60' : ''}`}>
      <div className="flex items-start gap-3">
        <div className="w-20 flex-shrink-0">
          <StatusBadge status={config.last_status} />
        </div>

        <div className="flex-1 min-w-0">
          <div className="flex items-center justify-between gap-4">
            <div className="flex items-center gap-2 min-w-0">
              <h3 className="text-base font-medium text-gray-900 truncate">{config.name}</h3>
              {isPaused && (
                <span className="text-xs px-1.5 py-0.5 bg-gray-200 text-gray-600 rounded flex-shrink-0">paused</span>
              )}
            </div>
            <div className="text-sm text-gray-500 flex-shrink-0">
              <span>{formatRelativeTime(config.last_executed_at)}</span>
              <span className="mx-1">|</span>
              <span>{isRunning ? 'running...' : isPaused ? 'paused' : formatNextRun(config.next_run_at)}</span>
            </div>
          </div>

          <p className="text-sm text-gray-500">
            {config.probe_type_name}
            {config.watcher_name && <span> @ {config.watcher_name}</span>}
          </p>

          {config.last_message && (
            <div className="mt-2">
              <button
                onClick={() => setIsExpanded(!isExpanded)}
                className="flex items-center gap-1 text-xs text-gray-500 hover:text-gray-700"
              >
                <svg
                  className={`w-3 h-3 transition-transform ${isExpanded ? 'rotate-90' : ''}`}
                  fill="none"
                  stroke="currentColor"
                  viewBox="0 0 24 24"
                >
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 5l7 7-7 7" />
                </svg>
                {isExpanded ? 'Hide details' : 'Show details'}
              </button>
              {isExpanded && (
                <div className="mt-2 text-sm text-gray-700 prose prose-sm max-w-none">
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
              )}
            </div>
          )}

          <div className="mt-3 flex gap-2">
            <button
              onClick={onClick}
              className="text-xs px-2 py-1 text-blue-600 hover:text-blue-800 hover:bg-blue-50 rounded"
            >
              Details
            </button>
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
                  disabled={isRunning}
                  className={`text-xs px-2 py-1 rounded ${
                    isRunning
                      ? 'text-blue-600 cursor-not-allowed'
                      : 'text-gray-600 hover:text-gray-900 hover:bg-gray-100'
                  }`}
                >
                  {isRunning ? 'Running...' : 'Rerun'}
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
        </div>
      </div>
    </div>
  );
}
