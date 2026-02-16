import { useEffect, useCallback, useRef } from 'react';
import { invoke } from '@tauri-apps/api/core';

const HEARTBEAT_INTERVAL = 30_000; // 30 seconds

export function usePresence(projectId: string | null) {
  const intervalRef = useRef<ReturnType<typeof setInterval> | null>(null);

  const updatePresence = useCallback(async (status: string, statusText = '') => {
    if (!projectId) return;
    try {
      await invoke('update_presence', { projectId, status, statusText });
    } catch {
      // Silently ignore presence errors
    }
  }, [projectId]);

  useEffect(() => {
    if (!projectId) return;

    // Set online on mount
    updatePresence('online');

    // Heartbeat
    intervalRef.current = setInterval(() => {
      updatePresence('online');
    }, HEARTBEAT_INTERVAL);

    // Set offline on unmount
    return () => {
      if (intervalRef.current) {
        clearInterval(intervalRef.current);
      }
      updatePresence('offline');
    };
  }, [projectId, updatePresence]);

  return { updatePresence };
}
