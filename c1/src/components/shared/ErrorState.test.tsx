import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { ErrorState } from './ErrorState';

describe('ErrorState', () => {
  it('renders error message', () => {
    render(<ErrorState message="Something went wrong" />);
    expect(screen.getByText('Something went wrong')).toBeInTheDocument();
  });

  it('renders detail when provided', () => {
    render(<ErrorState message="Error" detail="Connection refused" />);
    expect(screen.getByText('Connection refused')).toBeInTheDocument();
  });

  it('does not render detail when not provided', () => {
    const { container } = render(<ErrorState message="Error" />);
    expect(container.querySelector('.error-state__detail')).not.toBeInTheDocument();
  });

  it('renders retry button when onRetry provided', () => {
    const onRetry = vi.fn();
    render(<ErrorState message="Error" onRetry={onRetry} />);
    const button = screen.getByText('Retry');
    expect(button).toBeInTheDocument();
    fireEvent.click(button);
    expect(onRetry).toHaveBeenCalledOnce();
  });

  it('does not render retry button when onRetry is not provided', () => {
    render(<ErrorState message="Error" />);
    expect(screen.queryByText('Retry')).not.toBeInTheDocument();
  });

  it('renders warning icon', () => {
    const { container } = render(<ErrorState message="Error" />);
    expect(container.querySelector('.error-state__icon')).toBeInTheDocument();
  });
});
