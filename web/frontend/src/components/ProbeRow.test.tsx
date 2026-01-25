import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen } from '@testing-library/react';
import { ProbeRow } from './ProbeRow';
import type { ProbeConfig } from '../api/types';

function makeConfig(overrides: Partial<ProbeConfig> = {}): ProbeConfig {
  return {
    id: 1,
    probe_type_id: 1,
    probe_type_name: 'test-probe',
    name: 'Test Probe',
    enabled: true,
    arguments: {},
    interval: '1h',
    timeout_seconds: 60,
    notification_channels: [],
    created_at: '2024-01-01T00:00:00Z',
    updated_at: null,
    ...overrides,
  };
}

describe('ProbeRow overdue detection', () => {
  beforeEach(() => {
    // Mock current time to 2024-01-15T12:00:00Z
    vi.useFakeTimers();
    vi.setSystemTime(new Date('2024-01-15T12:00:00Z'));
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it('shows no overdue badge when probe is on schedule', () => {
    const config = makeConfig({
      next_run_at: '2024-01-15T12:30:00Z', // 30 minutes in the future
      last_status: 'ok',
    });

    render(<ProbeRow config={config} />);

    expect(screen.queryByText('overdue')).not.toBeInTheDocument();
  });

  it('shows no overdue badge when pending but within threshold', () => {
    const config = makeConfig({
      next_run_at: '2024-01-15T11:59:00Z', // 1 minute ago
      timeout_seconds: 60, // 3x = 180 seconds
      last_status: 'ok',
    });

    render(<ProbeRow config={config} />);

    expect(screen.queryByText('overdue')).not.toBeInTheDocument();
  });

  it('shows overdue badge when past 3x timeout threshold', () => {
    const config = makeConfig({
      next_run_at: '2024-01-15T11:55:00Z', // 5 minutes ago
      timeout_seconds: 60, // 3x = 180 seconds = 3 minutes
      last_status: 'ok',
    });

    render(<ProbeRow config={config} />);

    expect(screen.getByText('overdue')).toBeInTheDocument();
  });

  it('escalates ok status to warning when overdue', () => {
    const config = makeConfig({
      next_run_at: '2024-01-15T11:55:00Z', // 5 minutes ago
      timeout_seconds: 60,
      interval: '1h',
      last_status: 'ok',
    });

    render(<ProbeRow config={config} />);

    // The status indicator should be yellow (warning), not green (ok)
    const statusIndicator = document.querySelector('.bg-yellow-500');
    expect(statusIndicator).toBeInTheDocument();
  });

  it('escalates to critical when overdue by more than interval', () => {
    const config = makeConfig({
      next_run_at: '2024-01-15T10:00:00Z', // 2 hours ago
      timeout_seconds: 60,
      interval: '1h', // interval is 1 hour
      last_status: 'ok',
    });

    render(<ProbeRow config={config} />);

    // The status indicator should be red (critical)
    const statusIndicator = document.querySelector('.bg-red-500');
    expect(statusIndicator).toBeInTheDocument();
  });

  it('keeps warning status when overdue', () => {
    const config = makeConfig({
      next_run_at: '2024-01-15T11:55:00Z', // 5 minutes ago
      timeout_seconds: 60,
      last_status: 'warning',
    });

    render(<ProbeRow config={config} />);

    // Should still show warning (no escalation needed)
    const statusIndicator = document.querySelector('.bg-yellow-500');
    expect(statusIndicator).toBeInTheDocument();
  });

  it('keeps critical status when overdue', () => {
    const config = makeConfig({
      next_run_at: '2024-01-15T11:55:00Z', // 5 minutes ago
      timeout_seconds: 60,
      last_status: 'critical',
    });

    render(<ProbeRow config={config} />);

    // Should still show critical (no change)
    const statusIndicator = document.querySelector('.bg-red-500');
    expect(statusIndicator).toBeInTheDocument();
  });

  it('does not show overdue badge when paused', () => {
    const config = makeConfig({
      next_run_at: '2024-01-15T11:00:00Z', // 1 hour ago
      timeout_seconds: 60,
      enabled: false, // paused
      last_status: 'ok',
    });

    render(<ProbeRow config={config} />);

    expect(screen.queryByText('overdue')).not.toBeInTheDocument();
    expect(screen.getByText('paused')).toBeInTheDocument();
  });

  it('does not show overdue badge when running', () => {
    const config = makeConfig({
      next_run_at: '2024-01-15T11:00:00Z', // 1 hour ago
      timeout_seconds: 60,
      last_status: 'ok',
    });

    render(<ProbeRow config={config} isRunning={true} />);

    expect(screen.queryByText('overdue')).not.toBeInTheDocument();
  });
});
