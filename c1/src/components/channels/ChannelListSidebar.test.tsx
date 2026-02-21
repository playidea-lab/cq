import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import { ChannelListSidebar } from './ChannelListSidebar';
import type { Channel } from '../../types';

function makeChannel(id: string, name: string, channelType: Channel['channel_type']): Channel {
  return {
    id,
    name,
    project_id: 'proj-1',
    description: '',
    channel_type: channelType,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
  };
}

const noop = vi.fn();

describe('ChannelListSidebar section assignment', () => {
  it('places general channel in General section', () => {
    const channels = [makeChannel('1', 'announcements', 'general')];
    render(<ChannelListSidebar channels={channels} selectedChannel={null} onSelect={noop} onCreate={noop} />);
    expect(screen.getByText('announcements')).toBeInTheDocument();
  });

  it('places session channel in Sessions section', () => {
    const channels = [makeChannel('2', 'session-work', 'session')];
    render(<ChannelListSidebar channels={channels} selectedChannel={null} onSelect={noop} onCreate={noop} />);
    expect(screen.getByText('session-work')).toBeInTheDocument();
  });

  it('places project channel in Projects section', () => {
    const channels = [makeChannel('3', 'my-project', 'project')];
    render(<ChannelListSidebar channels={channels} selectedChannel={null} onSelect={noop} onCreate={noop} />);
    expect(screen.getByText('my-project')).toBeInTheDocument();
  });

  it('places dm channel in Direct section', () => {
    const channels = [makeChannel('4', 'alice', 'dm')];
    render(<ChannelListSidebar channels={channels} selectedChannel={null} onSelect={noop} onCreate={noop} />);
    expect(screen.getByText('alice')).toBeInTheDocument();
  });

  it('places auto channel in General section (auto maps to general)', () => {
    const channels = [makeChannel('5', 'auto-ch', 'auto')];
    render(<ChannelListSidebar channels={channels} selectedChannel={null} onSelect={noop} onCreate={noop} />);
    expect(screen.getByText('auto-ch')).toBeInTheDocument();
  });

  it('unmatched channel type falls back to General section', () => {
    // 'worker' type is not matched by any section → falls into general
    const channels = [makeChannel('6', 'worker-ch', 'worker')];
    render(<ChannelListSidebar channels={channels} selectedChannel={null} onSelect={noop} onCreate={noop} />);
    expect(screen.getByText('worker-ch')).toBeInTheDocument();
  });

  it('each channel appears only once across sections', () => {
    const channels = [
      makeChannel('1', 'general-ch', 'general'),
      makeChannel('2', 'session-ch', 'session'),
      makeChannel('3', 'project-ch', 'project'),
    ];
    render(<ChannelListSidebar channels={channels} selectedChannel={null} onSelect={noop} onCreate={noop} />);
    // Each channel name should appear exactly once
    expect(screen.getAllByText('general-ch')).toHaveLength(1);
    expect(screen.getAllByText('session-ch')).toHaveLength(1);
    expect(screen.getAllByText('project-ch')).toHaveLength(1);
  });

  it('shows section headers for all 5 sections', () => {
    render(<ChannelListSidebar channels={[]} selectedChannel={null} onSelect={noop} onCreate={noop} />);
    expect(screen.getByText('General')).toBeInTheDocument();
    expect(screen.getByText('Projects')).toBeInTheDocument();
    expect(screen.getByText('Knowledge')).toBeInTheDocument();
    expect(screen.getByText('Sessions')).toBeInTheDocument();
    expect(screen.getByText('Direct')).toBeInTheDocument();
  });

  it('shows empty state for sections with no channels', () => {
    render(<ChannelListSidebar channels={[]} selectedChannel={null} onSelect={noop} onCreate={noop} />);
    expect(screen.getByText('No general channels')).toBeInTheDocument();
    expect(screen.getByText('No sessions channels')).toBeInTheDocument();
  });
});
