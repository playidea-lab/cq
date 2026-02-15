import { useState, useEffect, useCallback } from 'react';
import { invoke } from '@tauri-apps/api/core';
import { listen } from '@tauri-apps/api/event';

export interface EventBusEvent {
  id: string;
  type: string;
  source: string;
  data: Record<string, unknown>;
  project_id?: string;
  correlation_id?: string;
  timestamp_ms: number;
}

export type EventBusConnectionStatus = 'connected' | 'disconnected' | 'reconnecting';

interface UseEventBusOptions {
  maxEvents?: number;
  filter?: string;
  autoConnect?: boolean;
}

export function useEventBus(options: UseEventBusOptions = {}) {
  const { maxEvents = 200, filter, autoConnect = true } = options;
  const [events, setEvents] = useState<EventBusEvent[]>([]);
  const [status, setStatus] = useState<EventBusConnectionStatus>('disconnected');
  const [paused, setPaused] = useState(false);

  useEffect(() => {
    const unlistenEvent = listen<EventBusEvent>('eventbus-event', (e) => {
      if (paused) return;
      const event = e.payload;
      if (filter && !event.type.includes(filter)) return;
      setEvents(prev => [event, ...prev].slice(0, maxEvents));
    });

    const unlistenStatus = listen<EventBusConnectionStatus>('eventbus-status', (e) => {
      setStatus(e.payload);
    });

    if (autoConnect) {
      invoke('eventbus_connect').catch(console.error);
    }

    return () => {
      unlistenEvent.then(fn => fn());
      unlistenStatus.then(fn => fn());
      invoke('eventbus_disconnect').catch(() => {});
    };
  }, [maxEvents, filter, paused, autoConnect]);

  const connect = useCallback(() => {
    invoke('eventbus_connect').catch(console.error);
  }, []);

  const disconnect = useCallback(() => {
    invoke('eventbus_disconnect').catch(console.error);
  }, []);

  const clear = useCallback(() => {
    setEvents([]);
  }, []);

  return {
    events,
    status,
    isConnected: status === 'connected',
    paused,
    setPaused,
    connect,
    disconnect,
    clear,
  };
}
