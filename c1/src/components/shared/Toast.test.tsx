import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent, act } from '@testing-library/react';
import { ToastContainer } from './Toast';
import type { ToastData } from './Toast';

describe('ToastContainer', () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it('renders nothing when toasts array is empty', () => {
    const { container } = render(<ToastContainer toasts={[]} onDismiss={vi.fn()} />);
    expect(container.querySelector('.toast-container')).not.toBeInTheDocument();
  });

  it('renders a success toast with check icon', () => {
    const toasts: ToastData[] = [{ id: '1', message: 'Saved!', type: 'success' }];
    render(<ToastContainer toasts={toasts} onDismiss={vi.fn()} />);
    expect(screen.getByText('Saved!')).toBeInTheDocument();
    expect(screen.getByRole('alert')).toHaveClass('toast--success');
  });

  it('renders an error toast', () => {
    const toasts: ToastData[] = [{ id: '1', message: 'Failed!', type: 'error' }];
    render(<ToastContainer toasts={toasts} onDismiss={vi.fn()} />);
    expect(screen.getByRole('alert')).toHaveClass('toast--error');
  });

  it('auto-dismisses after 3 seconds', () => {
    const onDismiss = vi.fn();
    const toasts: ToastData[] = [{ id: '1', message: 'Auto dismiss', type: 'info' }];
    render(<ToastContainer toasts={toasts} onDismiss={onDismiss} />);

    act(() => {
      vi.advanceTimersByTime(3000);
    });

    expect(onDismiss).toHaveBeenCalledWith('1');
  });

  it('dismisses on close button click', () => {
    const onDismiss = vi.fn();
    const toasts: ToastData[] = [{ id: '1', message: 'Close me', type: 'info' }];
    render(<ToastContainer toasts={toasts} onDismiss={onDismiss} />);

    fireEvent.click(screen.getByText('\u00D7'));
    expect(onDismiss).toHaveBeenCalledWith('1');
  });

  it('renders multiple toasts', () => {
    const toasts: ToastData[] = [
      { id: '1', message: 'First', type: 'success' },
      { id: '2', message: 'Second', type: 'error' },
    ];
    render(<ToastContainer toasts={toasts} onDismiss={vi.fn()} />);
    expect(screen.getByText('First')).toBeInTheDocument();
    expect(screen.getByText('Second')).toBeInTheDocument();
  });
});
