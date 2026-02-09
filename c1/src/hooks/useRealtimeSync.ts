import { useEffect, useState, useCallback, useRef } from 'react';
import { listen, type UnlistenFn } from '@tauri-apps/api/event';
import { invoke } from '@tauri-apps/api/core';

export type ConnectionStatus = 'disconnected' | 'connecting' | 'connected' | 'reconnecting';

export interface CloudChangeEvent {
  table: string;
  change_type: string; // INSERT, UPDATE, DELETE
  record: Record<string, unknown>;
  old_record?: Record<string, unknown>;
}

interface UseRealtimeSyncOptions {
  /** Called when any cloud change event is received */
  onUpdate?: (event: CloudChangeEvent) => void;
  /** Auto-connect on mount (default: true) */
  autoConnect?: boolean;
}

/**
 * Hook for subscribing to Supabase Realtime cloud updates via Tauri events.
 *
 * Usage:
 * ```tsx
 * const { status, connect, disconnect } = useRealtimeSync({
 *   onUpdate: (event) => {
 *     if (event.table === 'c4_tasks') refreshTasks();
 *   },
 * });
 * ```
 */
export function useRealtimeSync(options?: UseRealtimeSyncOptions) {
  const [status, setStatus] = useState<ConnectionStatus>('disconnected');
  const [lastEvent, setLastEvent] = useState<CloudChangeEvent | null>(null);
  const onUpdateRef = useRef(options?.onUpdate);
  onUpdateRef.current = options?.onUpdate;

  // Listen for connection status changes
  useEffect(() => {
    let unlisten: UnlistenFn | undefined;

    listen<ConnectionStatus>('realtime-status', (event) => {
      setStatus(event.payload);
    }).then((fn) => {
      unlisten = fn;
    });

    return () => {
      unlisten?.();
    };
  }, []);

  // Listen for cloud update events
  useEffect(() => {
    let unlisten: UnlistenFn | undefined;

    listen<CloudChangeEvent>('cloud-update', (event) => {
      setLastEvent(event.payload);
      onUpdateRef.current?.(event.payload);
    }).then((fn) => {
      unlisten = fn;
    });

    return () => {
      unlisten?.();
    };
  }, []);

  const connect = useCallback(async () => {
    try {
      await invoke('realtime_connect');
    } catch (err) {
      console.error('[realtime] connect failed:', err);
    }
  }, []);

  const disconnect = useCallback(async () => {
    try {
      await invoke('realtime_disconnect');
      setStatus('disconnected');
    } catch (err) {
      console.error('[realtime] disconnect failed:', err);
    }
  }, []);

  // Auto-connect on mount if requested
  useEffect(() => {
    if (options?.autoConnect !== false) {
      connect();
    }
    return () => {
      // Don't disconnect on unmount — let the connection persist
    };
  }, [connect, options?.autoConnect]);

  return {
    status,
    lastEvent,
    connect,
    disconnect,
    isConnected: status === 'connected',
  };
}
