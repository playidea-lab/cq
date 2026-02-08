import { useState, useCallback } from 'react';
import { invoke } from '@tauri-apps/api/core';
import type { ProjectState, TaskItem, TaskDetail, TaskEvent, ValidationResult } from '../types';

export function useDashboard() {
  const [state, setState] = useState<ProjectState | null>(null);
  const [tasks, setTasks] = useState<TaskItem[]>([]);
  const [selectedTask, setSelectedTask] = useState<TaskDetail | null>(null);
  const [timeline, setTimeline] = useState<TaskEvent[]>([]);
  const [validations, setValidations] = useState<ValidationResult[]>([]);
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
      const sorted = [...taskList].sort((a, b) => (order[a.status] ?? 9) - (order[b.status] ?? 9) || b.priority - a.priority);
      setTasks(sorted);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setLoading(false);
    }
  }, []);

  const loadTimeline = useCallback(async (projectPath: string) => {
    try {
      const events = await invoke<TaskEvent[]>('get_task_timeline', {
        path: projectPath,
      });
      setTimeline(events);
    } catch {
      // Timeline is non-critical; silently ignore errors
      setTimeline([]);
    }
  }, []);

  const loadValidations = useCallback(async (projectPath: string, taskId: string) => {
    try {
      const results = await invoke<ValidationResult[]>('get_validation_results', {
        path: projectPath,
        taskId,
      });
      setValidations(results);
    } catch {
      setValidations([]);
    }
  }, []);

  const clearValidations = useCallback(() => {
    setValidations([]);
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
    timeline,
    validations,
    loading,
    error,
    loadState,
    loadTimeline,
    loadValidations,
    clearValidations,
    loadTaskDetail,
    setSelectedTask,
  };
}
