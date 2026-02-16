import React, { createContext, useContext, useState, useEffect, useCallback } from 'react';
import { invoke } from '@tauri-apps/api/core';

interface Task {
  id: string;
  title: string;
  status: string;
}

interface TaskContextType {
  activeTask: Task | null;
  loading: boolean;
  refresh: () => Promise<void>;
}

const TaskContext = createContext<TaskContextType | undefined>(undefined);

export function TaskProvider({ children }: { children: React.ReactNode }) {
  const [activeTask, setActiveTask] = useState<Task | null>(null);
  const [loading, setLoading] = useState(false);

  const refresh = useCallback(async () => {
    setLoading(true);
    try {
      // In a real scenario, we would call a Tauri command that runs 'c4 status'
      // For now, we simulate with a mock or attempt to fetch via IPC if defined
      const status: any = await invoke('c4_status_json').catch(() => null);
      if (status && status.tasks) {
        const inProgress = status.tasks.find((t: any) => t.status === 'IN_PROGRESS');
        setActiveTask(inProgress || null);
      }
    } catch (err) {
      console.error('Failed to fetch task status:', err);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    refresh();
    const interval = setInterval(refresh, 10000); // Refresh every 10s
    return () => clearInterval(interval);
  }, [refresh]);

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
