import { useState, useEffect, useCallback } from 'react';
import { invoke } from '@tauri-apps/api/core';

export interface ChannelPin {
  id: string;
  channel_id: string;
  content: string;
  pin_type: string;
  version: number;
  created_by?: string;
  created_at: string;
}

export function useChannelPins(channelId: string | null) {
  const [pins, setPins] = useState<ChannelPin[]>([]);
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    if (!channelId) {
      setPins([]);
      return;
    }
    setLoading(true);
    invoke<ChannelPin[]>('list_channel_pins', { channelId })
      .then(setPins)
      .catch(console.error)
      .finally(() => setLoading(false));
  }, [channelId]);

  const createPin = useCallback(async (content: string, pinType: string = 'artifact') => {
    if (!channelId) return;
    const pin = await invoke<ChannelPin>('create_channel_pin', { channelId, content, pinType });
    setPins(prev => [pin, ...prev]);
    return pin;
  }, [channelId]);

  const deletePin = useCallback(async (pinId: string) => {
    await invoke('delete_channel_pin', { pinId });
    setPins(prev => prev.filter(p => p.id !== pinId));
  }, []);

  return { pins, loading, createPin, deletePin };
}
