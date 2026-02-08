import { useState, useCallback } from 'react';
import { invoke } from '@tauri-apps/api/core';
import type { SessionMeta, SessionPage, ProviderKind } from '../types';

const PAGE_SIZE = 50;

export function useSessions() {
  const [sessions, setSessions] = useState<SessionMeta[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [currentSession, setCurrentSession] = useState<SessionMeta | null>(null);
  const [page, setPage] = useState<SessionPage | null>(null);
  const [messagesLoading, setMessagesLoading] = useState(false);
  const [offset, setOffset] = useState(0);
  const [currentProvider, setCurrentProvider] = useState<ProviderKind>('claude_code');

  const listSessions = useCallback(async (projectPath: string, provider?: ProviderKind) => {
    setLoading(true);
    setError(null);
    setCurrentSession(null);
    setPage(null);
    const kind = provider || currentProvider;
    setCurrentProvider(kind);
    try {
      const result = await invoke<SessionMeta[]>('list_sessions_for_provider', {
        path: projectPath,
        provider: kind,
      });
      setSessions(result);
    } catch {
      // Fallback to legacy command for Claude Code
      try {
        const result = await invoke<SessionMeta[]>('list_sessions', { path: projectPath });
        setSessions(result);
      } catch (err2) {
        setError(err2 instanceof Error ? err2.message : String(err2));
      }
    } finally {
      setLoading(false);
    }
  }, [currentProvider]);

  const loadMessages = useCallback(async (session: SessionMeta) => {
    setCurrentSession(session);
    setMessagesLoading(true);
    setOffset(0);
    try {
      const result = await invoke<SessionPage>('get_provider_session_messages', {
        sessionPath: session.path,
        provider: currentProvider,
        offset: 0,
        limit: PAGE_SIZE,
      });
      setPage(result);
      setOffset(PAGE_SIZE);
    } catch {
      // Fallback to legacy command
      try {
        const result = await invoke<SessionPage>('get_session_messages', {
          sessionPath: session.path,
          offset: 0,
          limit: PAGE_SIZE,
        });
        setPage(result);
        setOffset(PAGE_SIZE);
      } catch (err2) {
        setError(err2 instanceof Error ? err2.message : String(err2));
      }
    } finally {
      setMessagesLoading(false);
    }
  }, [currentProvider]);

  const loadMore = useCallback(async () => {
    if (!currentSession || !page?.has_more) return;
    setMessagesLoading(true);
    try {
      const result = await invoke<SessionPage>('get_provider_session_messages', {
        sessionPath: currentSession.path,
        provider: currentProvider,
        offset,
        limit: PAGE_SIZE,
      });
      setPage(prev => prev ? {
        ...result,
        messages: [...prev.messages, ...result.messages],
      } : result);
      setOffset(prev => prev + PAGE_SIZE);
    } catch {
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
      } catch (err2) {
        setError(err2 instanceof Error ? err2.message : String(err2));
      }
    } finally {
      setMessagesLoading(false);
    }
  }, [currentSession, page, offset, currentProvider]);

  return {
    sessions,
    loading,
    error,
    currentSession,
    page,
    messagesLoading,
    currentProvider,
    listSessions,
    loadMessages,
    loadMore,
  };
}
