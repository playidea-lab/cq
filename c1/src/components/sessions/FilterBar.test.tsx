import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { FilterBar } from './FilterBar';

describe('FilterBar', () => {
  it('renders sort select with default value', () => {
    render(
      <FilterBar sortBy="date" onSortChange={vi.fn()} timeFilter="all" onTimeFilterChange={vi.fn()} />
    );
    const select = screen.getByDisplayValue('Date');
    expect(select).toBeInTheDocument();
  });

  it('calls onSortChange when sort selection changes', () => {
    const onSortChange = vi.fn();
    render(
      <FilterBar sortBy="date" onSortChange={onSortChange} timeFilter="all" onTimeFilterChange={vi.fn()} />
    );
    fireEvent.change(screen.getByDisplayValue('Date'), { target: { value: 'size' } });
    expect(onSortChange).toHaveBeenCalledWith('size');
  });

  it('renders time filter pills', () => {
    render(
      <FilterBar sortBy="date" onSortChange={vi.fn()} timeFilter="all" onTimeFilterChange={vi.fn()} />
    );
    expect(screen.getByText('All')).toBeInTheDocument();
    expect(screen.getByText('Today')).toBeInTheDocument();
    expect(screen.getByText('7d')).toBeInTheDocument();
    expect(screen.getByText('30d')).toBeInTheDocument();
  });

  it('highlights the active time filter pill', () => {
    const { container } = render(
      <FilterBar sortBy="date" onSortChange={vi.fn()} timeFilter="week" onTimeFilterChange={vi.fn()} />
    );
    const activePill = container.querySelector('.filter-bar__pill--active');
    expect(activePill).toBeInTheDocument();
    expect(activePill?.textContent).toBe('7d');
  });

  it('calls onTimeFilterChange when a pill is clicked', () => {
    const onTimeFilterChange = vi.fn();
    render(
      <FilterBar sortBy="date" onSortChange={vi.fn()} timeFilter="all" onTimeFilterChange={onTimeFilterChange} />
    );
    fireEvent.click(screen.getByText('Today'));
    expect(onTimeFilterChange).toHaveBeenCalledWith('today');
  });
});
