import { describe, it, expect } from 'vitest';
import { groupMessages } from './MessageList';
import type { C1Message } from '../../types';

function makeMsg(id: string, agentWorkId?: string | null): C1Message {
  return {
    id,
    channel_id: 'ch1',
    participant_id: 'user-1',
    content: `msg-${id}`,
    thread_id: null,
    metadata: null,
    agent_work_id: agentWorkId ?? null,
    created_at: '2026-01-01T00:00:00Z',
  };
}

describe('groupMessages', () => {
  it('returns empty array for empty input', () => {
    expect(groupMessages([])).toEqual([]);
  });

  it('returns single group for single non-agent message', () => {
    const msgs = [makeMsg('1')];
    const groups = groupMessages(msgs);
    expect(groups).toHaveLength(1);
    expect(groups[0].type).toBe('single');
    if (groups[0].type === 'single') {
      expect(groups[0].message.id).toBe('1');
    }
  });

  it('groups consecutive messages with same agent_work_id into a thread', () => {
    const msgs = [makeMsg('1', 'work-abc'), makeMsg('2', 'work-abc'), makeMsg('3', 'work-abc')];
    const groups = groupMessages(msgs);
    expect(groups).toHaveLength(1);
    expect(groups[0].type).toBe('thread');
    if (groups[0].type === 'thread') {
      expect(groups[0].workId).toBe('work-abc');
      expect(groups[0].messages).toHaveLength(3);
    }
  });

  it('does not group non-consecutive messages with same agent_work_id', () => {
    const msgs = [
      makeMsg('1', 'work-abc'),
      makeMsg('2'),
      makeMsg('3', 'work-abc'),
    ];
    const groups = groupMessages(msgs);
    expect(groups).toHaveLength(3);
    expect(groups[0].type).toBe('thread');
    expect(groups[1].type).toBe('single');
    expect(groups[2].type).toBe('thread');
    if (groups[0].type === 'thread') expect(groups[0].messages).toHaveLength(1);
    if (groups[2].type === 'thread') expect(groups[2].messages).toHaveLength(1);
  });

  it('handles mixed single and thread groups', () => {
    const msgs = [
      makeMsg('1'),
      makeMsg('2', 'work-x'),
      makeMsg('3', 'work-x'),
      makeMsg('4'),
      makeMsg('5', 'work-y'),
    ];
    const groups = groupMessages(msgs);
    expect(groups).toHaveLength(4);
    expect(groups[0].type).toBe('single');
    expect(groups[1].type).toBe('thread');
    if (groups[1].type === 'thread') {
      expect(groups[1].workId).toBe('work-x');
      expect(groups[1].messages).toHaveLength(2);
    }
    expect(groups[2].type).toBe('single');
    expect(groups[3].type).toBe('thread');
    if (groups[3].type === 'thread') {
      expect(groups[3].workId).toBe('work-y');
      expect(groups[3].messages).toHaveLength(1);
    }
  });

  it('treats null agent_work_id as single message', () => {
    const msgs = [makeMsg('1', null), makeMsg('2', null)];
    const groups = groupMessages(msgs);
    expect(groups).toHaveLength(2);
    expect(groups[0].type).toBe('single');
    expect(groups[1].type).toBe('single');
  });
});
