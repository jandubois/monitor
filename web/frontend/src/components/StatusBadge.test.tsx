import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import { StatusBadge } from './StatusBadge';

describe('StatusBadge', () => {
  it('renders "pending" when status is undefined', () => {
    render(<StatusBadge status={undefined} />);
    expect(screen.getByText('pending')).toBeInTheDocument();
  });

  it('renders the status text for each status type', () => {
    const statuses = ['ok', 'warning', 'critical', 'unknown'] as const;

    for (const status of statuses) {
      const { unmount } = render(<StatusBadge status={status} />);
      expect(screen.getByText(status)).toBeInTheDocument();
      unmount();
    }
  });

  it('applies correct color class for ok status', () => {
    render(<StatusBadge status="ok" />);
    const badge = screen.getByText('ok');
    expect(badge).toHaveClass('bg-green-500');
  });

  it('applies correct color class for warning status', () => {
    render(<StatusBadge status="warning" />);
    const badge = screen.getByText('warning');
    expect(badge).toHaveClass('bg-yellow-500');
  });

  it('applies correct color class for critical status', () => {
    render(<StatusBadge status="critical" />);
    const badge = screen.getByText('critical');
    expect(badge).toHaveClass('bg-red-500');
  });

  it('applies correct color class for unknown status', () => {
    render(<StatusBadge status="unknown" />);
    const badge = screen.getByText('unknown');
    expect(badge).toHaveClass('bg-gray-500');
  });

  it('applies small size classes', () => {
    render(<StatusBadge status="ok" size="sm" />);
    const badge = screen.getByText('ok');
    expect(badge).toHaveClass('text-xs');
  });

  it('applies medium size classes by default', () => {
    render(<StatusBadge status="ok" />);
    const badge = screen.getByText('ok');
    expect(badge).toHaveClass('text-sm');
  });

  it('applies large size classes', () => {
    render(<StatusBadge status="ok" size="lg" />);
    const badge = screen.getByText('ok');
    expect(badge).toHaveClass('text-base');
  });
});
