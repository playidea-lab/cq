import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import { StatusBadge } from './StatusBadge';

describe('StatusBadge', () => {
  it('renders status text with underscores replaced', () => {
    render(<StatusBadge status="in_progress" />);
    expect(screen.getByText('in progress')).toBeInTheDocument();
  });

  it('applies correct color class for task statuses', () => {
    const { container } = render(<StatusBadge status="done" />);
    expect(container.firstChild).toHaveClass('badge', 'badge--green');
  });

  it('applies correct color class for project statuses', () => {
    const { container } = render(<StatusBadge status="EXECUTE" />);
    expect(container.firstChild).toHaveClass('badge', 'badge--blue');
  });

  it('falls back to gray for unknown status', () => {
    const { container } = render(<StatusBadge status="unknown_status" />);
    expect(container.firstChild).toHaveClass('badge--gray');
    expect(screen.getByText('unknown status')).toBeInTheDocument();
  });

  it('applies additional className', () => {
    const { container } = render(<StatusBadge status="done" className="extra" />);
    expect(container.firstChild).toHaveClass('extra');
  });
});
