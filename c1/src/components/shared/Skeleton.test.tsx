import { describe, it, expect } from 'vitest';
import { render } from '@testing-library/react';
import { Skeleton } from './Skeleton';

describe('Skeleton', () => {
  it('renders line variant by default', () => {
    const { container } = render(<Skeleton />);
    expect(container.querySelector('.skeleton--line')).toBeInTheDocument();
  });

  it('renders card variant with internal elements', () => {
    const { container } = render(<Skeleton variant="card" />);
    expect(container.querySelector('.skeleton--card')).toBeInTheDocument();
    expect(container.querySelector('.skeleton__header')).toBeInTheDocument();
    expect(container.querySelector('.skeleton__body')).toBeInTheDocument();
  });

  it('renders list-item variant', () => {
    const { container } = render(<Skeleton variant="list-item" />);
    expect(container.querySelector('.skeleton--list-item')).toBeInTheDocument();
  });

  it('renders correct count of items', () => {
    const { container } = render(<Skeleton variant="list-item" count={4} />);
    const items = container.querySelectorAll('.skeleton--list-item');
    expect(items).toHaveLength(4);
  });

  it('applies custom className to container', () => {
    const { container } = render(<Skeleton className="custom-class" />);
    expect(container.querySelector('.skeleton-container.custom-class')).toBeInTheDocument();
  });
});
