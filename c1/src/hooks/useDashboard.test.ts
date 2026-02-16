import { describe, it, expect, vi, beforeEach } from 'vitest';
import { renderHook, act } from '@testing-library/react';
import { useDashboard } from './useDashboard';

vi.mock('@tauri-apps/api/core', () => ({
  invoke: vi.fn(),
}));

import { invoke } from '@tauri-apps/api/core';
const mockInvoke = vi.mocked(invoke);

const STATE_FIXTURE = {
  status: 'EXECUTE',
  project_id: 'test',
  workers: [],
  progress: { total: 5, done: 2, in_progress: 1, pending: 1, blocked: 1 },
};

describe('useDashboard', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('starts with loading=false and no state', () => {
    const { result } = renderHook(() => useDashboard());
    expect(result.current.loading).toBe(false);
    expect(result.current.state).toBeNull();
    expect(result.current.tasks).toEqual([]);
  });

  it('loads state and tasks on loadState call', async () => {
    mockInvoke.mockImplementation(async (cmd: string) => {
      if (cmd === 'get_project_state') return STATE_FIXTURE;
      if (cmd === 'get_tasks') return [
        { id: 'T-001', title: 'A', status: 'done', task_type: 'impl', priority: 0, assigned_to: null, domain: null, dependencies: [], validations: [] },
        { id: 'T-002', title: 'B', status: 'in_progress', task_type: 'impl', priority: 0, assigned_to: null, domain: null, dependencies: [], validations: [] },
      ];
      return null;
    });

    const { result } = renderHook(() => useDashboard());

    await act(async () => {
      await result.current.loadState('/test');
    });

    expect(result.current.state?.project_id).toBe('test');
    // in_progress tasks should be sorted first
    expect(result.current.tasks[0].status).toBe('in_progress');
    expect(result.current.tasks[1].status).toBe('done');
  });

  it('sets error on loadState failure', async () => {
    mockInvoke.mockRejectedValue(new Error('DB error'));

    const { result } = renderHook(() => useDashboard());

    await act(async () => {
      await result.current.loadState('/test');
    });

    expect(result.current.error).toBe('DB error');
    expect(result.current.state).toBeNull();
  });

  it('loads git graph without throwing on error', async () => {
    mockInvoke.mockRejectedValue(new Error('git graph fail'));

    const { result } = renderHook(() => useDashboard());

    await act(async () => {
      await result.current.loadGitGraph('/test');
    });

    // Git graph errors are silently handled
    expect(result.current.gitGraph).toEqual([]);
  });

  it('loads task detail', async () => {
    const detail = {
      id: 'T-001',
      title: 'Detail Task',
      status: 'done',
      dod: 'Complete feature',
      task_type: 'impl',
      priority: 0,
      scope: null,
      assigned_to: null,
      model: null,
      domain: null,
      dependencies: [],
      validations: ['test'],
      commit_sha: null,
      created_at: null,
    };

    mockInvoke.mockResolvedValue(detail);

    const { result } = renderHook(() => useDashboard());

    await act(async () => {
      await result.current.loadTaskDetail('/test', 'T-001');
    });

    expect(result.current.selectedTask?.title).toBe('Detail Task');
  });
});
