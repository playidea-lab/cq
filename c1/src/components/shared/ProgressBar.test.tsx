import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import { ProgressBar } from './ProgressBar';
import type { TaskProgress } from '../../types';

function makeProgress(overrides: Partial<TaskProgress> = {}): TaskProgress {
  return { total: 10, done: 0, in_progress: 0, pending: 0, blocked: 0, ...overrides };
}

describe('ProgressBar', () => {
  it('renders done/active/pending labels', () => {
    render(<ProgressBar progress={makeProgress({ done: 3, in_progress: 2, pending: 5 })} />);
    expect(screen.getByText('3 done')).toBeInTheDocument();
    expect(screen.getByText('2 active')).toBeInTheDocument();
    expect(screen.getByText('5 pending')).toBeInTheDocument();
  });

  it('renders blocked label only when blocked > 0', () => {
    const { rerender } = render(
      <ProgressBar progress={makeProgress({ done: 5, pending: 5 })} />
    );
    expect(screen.queryByText(/blocked/)).not.toBeInTheDocument();

    rerender(<ProgressBar progress={makeProgress({ done: 3, blocked: 2, pending: 5 })} />);
    expect(screen.getByText('2 blocked')).toBeInTheDocument();
  });

  it('calculates segment widths correctly', () => {
    const { container } = render(
      <ProgressBar progress={makeProgress({ done: 5, in_progress: 3, pending: 2 })} />
    );
    const segments = container.querySelectorAll('.progress-bar__segment');
    expect(segments[0]).toHaveStyle({ width: '50%' }); // done
    expect(segments[1]).toHaveStyle({ width: '30%' }); // in_progress
  });

  it('handles all-zero gracefully (no division by zero)', () => {
    const { container } = render(
      <ProgressBar progress={makeProgress()} />
    );
    const segments = container.querySelectorAll('.progress-bar__segment');
    expect(segments.length).toBeGreaterThan(0);
  });
});
