import { useState, useCallback, useEffect, useRef } from 'react';
import { invoke } from '@tauri-apps/api/core';
import { useRealtimeSync } from './useRealtimeSync';
import type { C1Message, C1MessagePage } from '../types';

const PAGE_SIZE = 30;

// Runtime type guard for realtime message records
function isValidMessage(record: unknown): record is C1Message {
  if (!record || typeof record !== 'object') return false;
  const r = record as Record<string, unknown>;
  return (
    typeof r.id === 'string' &&
    typeof r.content === 'string' &&
    typeof r.channel_id === 'string'
  );
}

export function useMessages(channelId: string | null) {
  const [messages, setMessages] = useState<C1Message[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [hasMore, setHasMore] = useState(false);
  const [total, setTotal] = useState(0);
  const offsetRef = useRef(0);

  const fetchMessages = useCallback(async () => {
    if (!channelId) return;
    setLoading(true);
    setError(null);
    offsetRef.current = 0;
    try {
      const result = await invoke<C1MessagePage>('get_channel_messages', {
        channelId,
        offset: 0,
        limit: PAGE_SIZE,
      });
      // API returns newest first; reverse so oldest is first for chat display
      setMessages(result.messages.slice().reverse());
      setHasMore(result.has_more);
      setTotal(result.total);
      offsetRef.current = PAGE_SIZE;

      // Mark as read
      invoke('mark_read', { channelId }).catch(() => {});
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setLoading(false);
    }
  }, [channelId]);

  // Load when channel changes
  useEffect(() => {
    if (channelId) {
      fetchMessages();
    } else {
      setMessages([]);
      setHasMore(false);
      setTotal(0);
    }
  }, [channelId, fetchMessages]);

  // Realtime: append new messages for the current channel
  useRealtimeSync({
    onUpdate: (event) => {
      if (
        event.table === 'c1_messages' &&
        event.change_type === 'INSERT' &&
        channelId
      ) {
        // Runtime validation for realtime record
        if (!isValidMessage(event.record)) {
          console.warn('Invalid message record from realtime event:', event.record);
          return;
        }
        const record = event.record as C1Message;
        if (record.channel_id === channelId) {
          setMessages(prev => {
            // Deduplicate
            if (prev.some(m => m.id === record.id)) return prev;
            return [...prev, record];
          });
          setTotal(prev => prev + 1);
          // Mark as read on new message
          invoke('mark_read', { channelId }).catch(() => {});
        }
      }
    },
    autoConnect: !!channelId,
  });

  const loadMore = useCallback(async () => {
    if (!channelId || !hasMore || loading) return;
    setLoading(true);
    try {
      const result = await invoke<C1MessagePage>('get_channel_messages', {
        channelId,
        offset: offsetRef.current,
        limit: PAGE_SIZE,
      });
      // Older messages go to the front (API returns newest-first, we reverse)
      setMessages(prev => [...result.messages.slice().reverse(), ...prev]);
      setHasMore(result.has_more);
      offsetRef.current += PAGE_SIZE;
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setLoading(false);
    }
  }, [channelId, hasMore, loading]);

  const sendMessage = useCallback(async (
    content: string,
    threadId?: string,
    metadata?: Record<string, unknown>,
  ): Promise<C1Message | null> => {
    if (!channelId) return null;
    try {
      const msg = await invoke<C1Message>('send_message', {
        channelId,
        content,
        threadId: threadId ?? null,
        metadata: metadata ?? null,
      });
      // Optimistic: append immediately (realtime may also fire)
      setMessages(prev => {
        if (prev.some(m => m.id === msg.id)) return prev;
        return [...prev, msg];
      });
      setTotal(prev => prev + 1);
      return msg;
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
      return null;
    }
  }, [channelId]);

  return {
    messages,
    loading,
    error,
    hasMore,
    total,
    loadMore,
    sendMessage,
    refresh: fetchMessages,
  };
}
