import { useState, useCallback, useEffect } from 'react';
import { invoke } from '@tauri-apps/api/core';
import { useRealtimeSync } from './useRealtimeSync';
import type { Channel } from '../types';

export function useChannels(projectId: string | null) {
  const [channels, setChannels] = useState<Channel[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [selectedChannel, setSelectedChannel] = useState<Channel | null>(null);

  const fetchChannels = useCallback(async () => {
    if (!projectId) return;
    setLoading(true);
    setError(null);
    try {
      const result = await invoke<Channel[]>('list_channels', { projectId });
      setChannels(result);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setLoading(false);
    }
  }, [projectId]);

  // Initial load
  useEffect(() => {
    fetchChannels();
  }, [fetchChannels]);

  // Realtime: refresh channel list on INSERT/UPDATE to c1_channels
  useRealtimeSync({
    onUpdate: (event) => {
      if (event.table === 'c1_channels') {
        fetchChannels();
      }
    },
    autoConnect: !!projectId,
  });

  const createChannel = useCallback(async (
    name: string,
    description: string,
    channelType: string,
  ): Promise<Channel | null> => {
    if (!projectId) return null;
    try {
      const channel = await invoke<Channel>('create_channel', {
        projectId,
        name,
        description,
        channelType,
      });
      // Optimistic: add to list immediately
      setChannels(prev => [...prev, channel].sort((a, b) => a.name.localeCompare(b.name)));
      return channel;
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
      return null;
    }
  }, [projectId]);

  const selectChannel = useCallback((channel: Channel | null) => {
    setSelectedChannel(channel);
  }, []);

  return {
    channels,
    loading,
    error,
    selectedChannel,
    fetchChannels,
    createChannel,
    selectChannel,
  };
}
