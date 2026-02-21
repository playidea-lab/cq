import { describe, it, expect, vi } from 'vitest';
import { renderHook } from '@testing-library/react';
import { useSessionChannels } from './useSessionChannels';
import type { Channel } from '../types';

vi.mock('./useChannels', () => ({
  useChannels: vi.fn(),
}));

import { useChannels } from './useChannels';
const mockUseChannels = vi.mocked(useChannels);

function makeChannel(id: string, channelType: Channel['channel_type']): Channel {
  return {
    id,
    name: `ch-${id}`,
    project_id: 'proj-1',
    description: '',
    channel_type: channelType,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
  };
}

describe('useSessionChannels', () => {
  it('returns only session channels', () => {
    mockUseChannels.mockReturnValue({
      channels: [
        makeChannel('1', 'general'),
        makeChannel('2', 'session'),
        makeChannel('3', 'project'),
        makeChannel('4', 'session'),
      ],
      loading: false,
      error: null,
      selectedChannel: null,
      fetchChannels: vi.fn(),
      createChannel: vi.fn(),
      selectChannel: vi.fn(),
    });

    const { result } = renderHook(() => useSessionChannels('proj-1'));
    expect(result.current.sessionChannels).toHaveLength(2);
    expect(result.current.sessionChannels.every((ch) => ch.channel_type === 'session')).toBe(true);
  });

  it('returns empty array when no session channels exist', () => {
    mockUseChannels.mockReturnValue({
      channels: [makeChannel('1', 'general'), makeChannel('2', 'project')],
      loading: false,
      error: null,
      selectedChannel: null,
      fetchChannels: vi.fn(),
      createChannel: vi.fn(),
      selectChannel: vi.fn(),
    });

    const { result } = renderHook(() => useSessionChannels('proj-1'));
    expect(result.current.sessionChannels).toHaveLength(0);
  });

  it('forwards loading state from useChannels', () => {
    mockUseChannels.mockReturnValue({
      channels: [],
      loading: true,
      error: null,
      selectedChannel: null,
      fetchChannels: vi.fn(),
      createChannel: vi.fn(),
      selectChannel: vi.fn(),
    });

    const { result } = renderHook(() => useSessionChannels('proj-1'));
    expect(result.current.loading).toBe(true);
  });

  it('passes projectId through to useChannels', () => {
    mockUseChannels.mockReturnValue({
      channels: [],
      loading: false,
      error: null,
      selectedChannel: null,
      fetchChannels: vi.fn(),
      createChannel: vi.fn(),
      selectChannel: vi.fn(),
    });

    renderHook(() => useSessionChannels('my-project'));
    expect(mockUseChannels).toHaveBeenCalledWith('my-project');
  });

  it('handles null projectId', () => {
    mockUseChannels.mockReturnValue({
      channels: [],
      loading: false,
      error: null,
      selectedChannel: null,
      fetchChannels: vi.fn(),
      createChannel: vi.fn(),
      selectChannel: vi.fn(),
    });

    const { result } = renderHook(() => useSessionChannels(null));
    expect(result.current.sessionChannels).toHaveLength(0);
    expect(mockUseChannels).toHaveBeenCalledWith(null);
  });
});
