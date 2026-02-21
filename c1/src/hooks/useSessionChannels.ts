import { useMemo } from 'react';
import { useChannels } from './useChannels';
import type { Channel } from '../types';

export function useSessionChannels(projectId: string | null): {
  sessionChannels: Channel[];
  loading: boolean;
} {
  const { channels, loading } = useChannels(projectId);
  const sessionChannels = useMemo(
    () => channels.filter((ch) => ch.channel_type === 'session'),
    [channels],
  );
  return { sessionChannels, loading };
}
