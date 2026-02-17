import React, { createContext, useContext, useState, useCallback } from 'react';
import { invoke } from '@tauri-apps/api/core';

// Backend response from Rust get_tasks command
interface TaskItem {
  task_id: string;
  title: string;
  status: 'IN_PROGRESS' | 'RUNNING' | 'PENDING' | 'COMPLETED' | 'BLOCKED' | string;
  worker_id?: string;
  dependencies?: string;
  domain?: string;
  priority?: number;
}

// UI-friendly Task interface
interface Task {
  id: string;
  title: string;
  status: string;
}

// Convert TaskItem to Task
function toTask(item: TaskItem): Task {
  return {
    id: item.task_id,
    title: item.title,
    status: item.status,
  };
}

interface TaskContextType {
  activeTask: Task | null;
  loading: boolean;
  refresh: (path: string | null) => Promise<void>;
}

const TaskContext = createContext<TaskContextType | undefined>(undefined);

export function TaskProvider({ children }: { children: React.ReactNode }) {
  const [activeTask, setActiveTask] = useState<Task | null>(null);
  const [loading, setLoading] = useState(false);

  const refresh = useCallback(async (path: string | null) => {
    if (!path) {
      setActiveTask(null);
      return;
    }

    // Show loading only on initial load to avoid UI flicker
    const isInitialLoad = activeTask === null;
    if (isInitialLoad) {
      setLoading(true);
    }

    try {
      // Call 'get_tasks' command with 'path' argument
      const tasks = await invoke<TaskItem[]>('get_tasks', { path }).catch(() => []);

      // Find the first task with status 'IN_PROGRESS' (or 'RUNNING')
      const inProgress = tasks.find(t =>
        t.status === 'IN_PROGRESS' || t.status === 'RUNNING'
      );

      setActiveTask(inProgress ? toTask(inProgress) : null);
    } catch (err) {
      console.error('Failed to fetch tasks:', err);
    } finally {
      if (isInitialLoad) {
        setLoading(false);
      }
    }
  }, [activeTask]);

  return (
    <TaskContext.Provider value={{ activeTask, loading, refresh }}>
      {children}
    </TaskContext.Provider>
  );
}

export function useTask() {
  const context = useContext(TaskContext);
  if (context === undefined) {
    throw new Error('useTask must be used within a TaskProvider');
  }
  return context;
}