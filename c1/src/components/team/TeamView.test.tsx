import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor, fireEvent } from '@testing-library/react';
import { TeamView } from './TeamView';

vi.mock('@tauri-apps/api/core', () => ({
  invoke: vi.fn(),
}));

vi.mock('@tauri-apps/api/event', () => ({
  listen: vi.fn(() => Promise.resolve(() => {})),
}));

import { invoke } from '@tauri-apps/api/core';
const mockInvoke = vi.mocked(invoke);

const PROJECT_FIXTURE = {
  id: 'proj-1',
  name: 'My Project',
  owner_email: 'user@example.com',
  task_count: 10,
  done_count: 5,
  status: 'EXECUTE',
  last_updated: '2026-01-15T10:00:00Z',
};

describe('TeamView', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('shows loading state initially', () => {
    mockInvoke.mockImplementation(() => new Promise(() => {})); // never resolves
    render(<TeamView />);
    expect(screen.getByText('Loading team projects...')).toBeInTheDocument();
  });

  it('renders project list when data loads', async () => {
    mockInvoke.mockResolvedValue([PROJECT_FIXTURE]);
    render(<TeamView />);

    await waitFor(() => {
      expect(screen.getByText('My Project')).toBeInTheDocument();
    });
    expect(screen.getByText('user@example.com')).toBeInTheDocument();
    expect(screen.getByText('5/10 tasks')).toBeInTheDocument();
    expect(screen.getByText('1 projects')).toBeInTheDocument();
  });

  it('shows error state with retry button on failure', async () => {
    mockInvoke.mockRejectedValue(new Error('Network error'));
    render(<TeamView />);

    await waitFor(() => {
      expect(screen.getByText('Failed to load team projects')).toBeInTheDocument();
    });
    expect(screen.getByText('Network error')).toBeInTheDocument();
    expect(screen.getByText('Retry')).toBeInTheDocument();
  });

  it('shows empty state when no projects exist', async () => {
    mockInvoke.mockResolvedValue([]);
    render(<TeamView />);

    await waitFor(() => {
      expect(screen.getByText('No Team Projects')).toBeInTheDocument();
    });
  });

  it('loads remote dashboard when project is selected', async () => {
    mockInvoke.mockImplementation(async (cmd: string) => {
      if (cmd === 'cloud_get_team_projects') return [PROJECT_FIXTURE];
      if (cmd === 'cloud_get_remote_dashboard') {
        return {
          status: 'EXECUTE',
          project_id: 'proj-1',
          workers: [],
          progress: { total: 10, done: 5, in_progress: 2, pending: 2, blocked: 1 },
        };
      }
      return null;
    });

    render(<TeamView />);

    await waitFor(() => {
      expect(screen.getByText('My Project')).toBeInTheDocument();
    });

    fireEvent.click(screen.getByText('My Project'));

    await waitFor(() => {
      expect(screen.getByText('Done')).toBeInTheDocument();
      expect(screen.getByText('Active')).toBeInTheDocument();
    });
  });

  it('shows knowledge tab with docs when selected', async () => {
    mockInvoke.mockImplementation(async (cmd: string) => {
      if (cmd === 'cloud_get_team_projects') return [PROJECT_FIXTURE];
      if (cmd === 'cloud_get_remote_dashboard') {
        return {
          status: 'EXECUTE',
          project_id: 'proj-1',
          workers: [],
          progress: { total: 10, done: 5, in_progress: 2, pending: 2, blocked: 1 },
        };
      }
      if (cmd === 'cloud_get_knowledge_docs') {
        return [
          {
            doc_id: 'exp-001',
            doc_type: 'experiment',
            title: 'Test Experiment',
            domain: 'ml',
            tags: ['pytorch'],
            body: '# Results',
            content_hash: 'abc',
            version: 1,
            created_at: '2026-02-10T00:00:00Z',
            updated_at: '2026-02-10T00:00:00Z',
          },
        ];
      }
      return null;
    });

    render(<TeamView />);

    await waitFor(() => {
      expect(screen.getByText('My Project')).toBeInTheDocument();
    });

    fireEvent.click(screen.getByText('My Project'));

    await waitFor(() => {
      expect(screen.getByText('Knowledge (1)')).toBeInTheDocument();
    });

    fireEvent.click(screen.getByText('Knowledge (1)'));

    await waitFor(() => {
      expect(screen.getByText('Test Experiment')).toBeInTheDocument();
      expect(screen.getByText('experiment')).toBeInTheDocument();
      expect(screen.getByText('pytorch')).toBeInTheDocument();
    });
  });
});
