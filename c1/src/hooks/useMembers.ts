import { useState, useCallback, useEffect, useRef } from 'react';
import { invoke } from '@tauri-apps/api/core';
import { useRealtimeSync } from './useRealtimeSync';
import type { C1Member } from '../types';

export function useMembers(projectId: string | null) {
  const [members, setMembers] = useState<C1Member[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const pollRef = useRef<ReturnType<typeof setInterval> | null>(null);

  const fetchMembers = useCallback(async () => {
    if (!projectId) return;
    setLoading(true);
    setError(null);
    try {
      const result = await invoke<C1Member[]>('list_members', { projectId });
      setMembers(result);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setLoading(false);
    }
  }, [projectId]);

  useEffect(() => {
    fetchMembers();
  }, [fetchMembers]);

  // Polling fallback: refresh every 5s in case Realtime misses updates
  useEffect(() => {
    if (!projectId) return;
    pollRef.current = setInterval(fetchMembers, 5000);
    return () => {
      if (pollRef.current) clearInterval(pollRef.current);
    };
  }, [projectId, fetchMembers]);

  // Realtime: refresh on c1_members changes
  useRealtimeSync({
    onUpdate: (event) => {
      if (event.table === 'c1_members') {
        fetchMembers();
      }
    },
    autoConnect: !!projectId,
  });

  const getMember = useCallback((memberId: string): C1Member | undefined => {
    return members.find(m => m.id === memberId);
  }, [members]);

  return {
    members,
    loading,
    error,
    fetchMembers,
    getMember,
  };
}
