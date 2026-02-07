import { useState, useCallback } from 'react';
import { invoke } from '@tauri-apps/api/core';
import type { SessionMeta, SessionPage } from '../types';

const PAGE_SIZE = 50;

export function useSessions() {
  const [sessions, setSessions] = useState<SessionMeta[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [currentSession, setCurrentSession] = useState<SessionMeta | null>(null);
  const [page, setPage] = useState<SessionPage | null>(null);
  const [messagesLoading, setMessagesLoading] = useState(false);
  const [offset, setOffset] = useState(0);

  const listSessions = useCallback(async (projectPath: string) => {
    setLoading(true);
    setError(null);
    try {
      const result = await invoke<SessionMeta[]>('list_sessions', { path: projectPath });
      setSessions(result);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setLoading(false);
    }
  }, []);

  const loadMessages = useCallback(async (session: SessionMeta) => {
    setCurrentSession(session);
    setMessagesLoading(true);
    setOffset(0);
    try {
      const result = await invoke<SessionPage>('get_session_messages', {
        sessionPath: session.path,
        offset: 0,
        limit: PAGE_SIZE,
      });
      setPage(result);
      setOffset(PAGE_SIZE);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setMessagesLoading(false);
    }
  }, []);

  const loadMore = useCallback(async () => {
    if (!currentSession || !page?.has_more) return;
    setMessagesLoading(true);
    try {
      const result = await invoke<SessionPage>('get_session_messages', {
        sessionPath: currentSession.path,
        offset,
        limit: PAGE_SIZE,
      });
      setPage(prev => prev ? {
        ...result,
        messages: [...prev.messages, ...result.messages],
      } : result);
      setOffset(prev => prev + PAGE_SIZE);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setMessagesLoading(false);
    }
  }, [currentSession, page, offset]);

  return {
    sessions,
    loading,
    error,
    currentSession,
    page,
    messagesLoading,
    listSessions,
    loadMessages,
    loadMore,
  };
}
