import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/react';

// Mock next/navigation
vi.mock('next/navigation', () => ({
  usePathname: () => '/',
}));

// Mock next/link
vi.mock('next/link', () => ({
  default: ({ children, href }: { children: React.ReactNode; href: string }) => (
    <a href={href}>{children}</a>
  ),
}));

import Header from '@/app/components/Header';
import ProjectCard from '@/app/components/ProjectCard';

describe('Header', () => {
  it('renders logo', () => {
    render(<Header />);
    expect(screen.getByText('C4')).toBeDefined();
    expect(screen.getByText('Cloud')).toBeDefined();
  });

  it('renders navigation links', () => {
    render(<Header />);
    expect(screen.getByText('Chat')).toBeDefined();
    expect(screen.getByText('Projects')).toBeDefined();
  });

  it('shows connection status', () => {
    render(<Header />);
    expect(screen.getByText('Connected')).toBeDefined();
  });
});

describe('ProjectCard', () => {
  const mockProject = {
    id: 'test-project',
    name: 'Test Project',
    status: 'EXECUTE',
    tasks_done: 10,
    tasks_pending: 5,
    created_at: '2024-01-01T00:00:00Z',
  };

  it('renders project name', () => {
    render(<ProjectCard project={mockProject} />);
    expect(screen.getByText('Test Project')).toBeDefined();
  });

  it('renders status badge', () => {
    render(<ProjectCard project={mockProject} />);
    expect(screen.getByText('EXECUTE')).toBeDefined();
  });

  it('calculates progress correctly', () => {
    render(<ProjectCard project={mockProject} />);
    // 10 / (10 + 5) = 66.67% -> rounds to 67%
    expect(screen.getByText('67%')).toBeDefined();
  });

  it('displays task counts', () => {
    render(<ProjectCard project={mockProject} />);
    expect(screen.getByText('10')).toBeDefined(); // done
    expect(screen.getByText('5')).toBeDefined();  // pending
  });

  it('links to project detail page', () => {
    render(<ProjectCard project={mockProject} />);
    const link = screen.getByRole('link');
    expect(link.getAttribute('href')).toBe('/projects/test-project');
  });
});
