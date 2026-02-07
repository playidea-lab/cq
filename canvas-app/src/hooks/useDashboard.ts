import { useState, useCallback } from 'react';
import { invoke } from '@tauri-apps/api/core';
import type { ProjectState, TaskItem, TaskDetail } from '../types';

export function useDashboard() {
  const [state, setState] = useState<ProjectState | null>(null);
  const [tasks, setTasks] = useState<TaskItem[]>([]);
  const [selectedTask, setSelectedTask] = useState<TaskDetail | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const loadState = useCallback(async (projectPath: string) => {
    setLoading(true);
    setError(null);
    try {
      const [projectState, taskList] = await Promise.all([
        invoke<ProjectState>('get_project_state', { path: projectPath }),
        invoke<TaskItem[]>('get_tasks', { path: projectPath }),
      ]);
      setState(projectState);
      // Sort: in_progress first, then pending, then done
      const order: Record<string, number> = { in_progress: 0, pending: 1, blocked: 2, done: 3 };
      taskList.sort((a, b) => (order[a.status] ?? 9) - (order[b.status] ?? 9) || b.priority - a.priority);
      setTasks(taskList);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setLoading(false);
    }
  }, []);

  const loadTaskDetail = useCallback(async (projectPath: string, taskId: string) => {
    try {
      const detail = await invoke<TaskDetail | null>('get_task_detail', {
        path: projectPath,
        taskId,
      });
      setSelectedTask(detail);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    }
  }, []);

  return {
    state,
    tasks,
    selectedTask,
    loading,
    error,
    loadState,
    loadTaskDetail,
    setSelectedTask,
  };
}
