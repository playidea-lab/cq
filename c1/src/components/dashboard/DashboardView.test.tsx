import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import { DashboardView } from './DashboardView';

vi.mock('@tauri-apps/api/core', () => ({
  invoke: vi.fn(),
}));

vi.mock('@tauri-apps/api/event', () => ({
  listen: vi.fn(() => Promise.resolve(() => {})),
}));

import { invoke } from '@tauri-apps/api/core';
const mockInvoke = vi.mocked(invoke);

const STATE_FIXTURE = {
  status: 'EXECUTE',
  project_id: 'test-project',
  workers: [],
  progress: { total: 10, done: 4, in_progress: 3, pending: 2, blocked: 1 },
};

const TASK_FIXTURE = {
  id: 'T-001-0',
  title: 'Implement feature',
  status: 'in_progress',
  task_type: 'impl',
  priority: 0,
  assigned_to: 'worker-1',
  domain: 'frontend',
  dependencies: [],
  validations: [],
};

function setupMock(overrides: Record<string, unknown> = {}) {
  mockInvoke.mockImplementation(async (cmd: string) => {
    if (cmd in overrides) return overrides[cmd];
    switch (cmd) {
      case 'get_project_state': return STATE_FIXTURE;
      case 'get_tasks': return [TASK_FIXTURE];
      case 'get_task_timeline': return [];
      case 'get_provider_token_usage': return null;
      case 'list_providers': return [];
      default: return null;
    }
  });
}

describe('DashboardView', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('shows skeleton loading state', () => {
    mockInvoke.mockImplementation(() => new Promise(() => {})); // never resolves
    const { container } = render(<DashboardView projectPath="/test" />);
    expect(container.querySelector('.skeleton-container')).toBeInTheDocument();
  });

  it('shows error state with retry', async () => {
    mockInvoke.mockRejectedValue(new Error('DB not found'));
    render(<DashboardView projectPath="/test" />);

    await waitFor(() => {
      expect(screen.getByText('Failed to load project state')).toBeInTheDocument();
    });
    expect(screen.getByText('DB not found')).toBeInTheDocument();
    expect(screen.getByText('Retry')).toBeInTheDocument();
  });

  it('shows empty state when no C4 data found', async () => {
    setupMock({ get_project_state: null });
    render(<DashboardView projectPath="/test" />);

    await waitFor(() => {
      expect(screen.getByText('No C4 project data found')).toBeInTheDocument();
    });
  });

  it('renders project state with status and tasks', async () => {
    setupMock();
    render(<DashboardView projectPath="/test" />);

    await waitFor(() => {
      expect(screen.getByText('test-project')).toBeInTheDocument();
    });
    expect(screen.getByText('Tasks (1)')).toBeInTheDocument();
    expect(screen.getByText('Implement feature')).toBeInTheDocument();
  });

  it('renders sync button', async () => {
    setupMock();
    render(<DashboardView projectPath="/test" />);

    await waitFor(() => {
      expect(screen.getByText('Sync to Cloud')).toBeInTheDocument();
    });
  });

  it('shows task detail placeholder when no task selected', async () => {
    setupMock();
    render(<DashboardView projectPath="/test" />);

    await waitFor(() => {
      expect(screen.getByText('Select a task to view details')).toBeInTheDocument();
    });
  });
});
