import { useEffect, useRef } from 'react';
import { MessageBubble } from './MessageBubble';
import { AgentThread } from './AgentThread';
import { Skeleton } from '../shared/Skeleton';
import type { C1Message, C1Member } from '../../types';

interface MessageListProps {
  messages: C1Message[];
  loading: boolean;
  hasMore: boolean;
  onLoadMore: () => void;
  getMember?: (memberId: string) => C1Member | undefined;
  onAction?: (id: string, label: string) => void;
}

// Message grouping types
export type MessageGroup =
  | { type: 'single'; message: C1Message }
  | { type: 'thread'; workId: string; messages: C1Message[] };

export function groupMessages(messages: C1Message[]): MessageGroup[] {
  const groups: MessageGroup[] = [];
  let i = 0;
  while (i < messages.length) {
    const msg = messages[i];
    const workId = msg.agent_work_id;
    if (workId) {
      // Collect all consecutive messages with the same agent_work_id
      const threadMessages: C1Message[] = [msg];
      while (i + 1 < messages.length && messages[i + 1].agent_work_id === workId) {
        i++;
        threadMessages.push(messages[i]);
      }
      groups.push({ type: 'thread', workId, messages: threadMessages });
    } else {
      groups.push({ type: 'single', message: msg });
    }
    i++;
  }
  return groups;
}

const COMPLETE_STATUSES = new Set(['completed', 'failed', 'cancelled']);

function isThreadComplete(messages: C1Message[]): boolean {
  const lastMsg = messages[messages.length - 1];
  const status = lastMsg?.metadata?.status as string | undefined;
  return COMPLETE_STATUSES.has(status ?? '');
}

function getAgentName(messages: C1Message[], getMember?: (id: string) => C1Member | undefined): string {
  const firstMsg = messages[0];
  if (!firstMsg) return 'Agent';
  if (firstMsg.member_id && getMember) {
    const member = getMember(firstMsg.member_id);
    if (member?.display_name) return member.display_name;
  }
  // Fallback: parse participant_id
  const pid = firstMsg.participant_id;
  if (pid.startsWith('worker-')) return pid;
  if (pid.startsWith('agent-')) return pid;
  return 'Agent';
}

export function MessageList({ messages, loading, hasMore, onLoadMore, getMember, onAction }: MessageListProps) {
  const bottomRef = useRef<HTMLDivElement>(null);
  const prevCountRef = useRef(messages.length);

  // Auto-scroll to bottom when new messages arrive (not when loading older)
  useEffect(() => {
    if (messages.length > prevCountRef.current) {
      bottomRef.current?.scrollIntoView({ behavior: 'smooth' });
    }
    prevCountRef.current = messages.length;
  }, [messages.length]);

  if (loading && messages.length === 0) {
    return (
      <div className="message-list">
        <Skeleton variant="list-item" count={5} />
      </div>
    );
  }

  if (messages.length === 0) {
    return (
      <div className="message-list">
        <div className="chat-panel__empty">No messages yet. Start a conversation!</div>
      </div>
    );
  }

  const groups = groupMessages(messages);

  return (
    <div className="message-list">
      {hasMore && (
        <button
          className="message-list__load-more"
          onClick={onLoadMore}
          disabled={loading}
        >
          {loading ? 'Loading...' : 'Load older messages'}
        </button>
      )}
      {groups.map((group, idx) => {
        if (group.type === 'single') {
          return (
            <MessageBubble
              key={group.message.id}
              message={group.message}
              member={group.message.member_id && getMember ? getMember(group.message.member_id) : undefined}
              onAction={onAction}
            />
          );
        }
        // type === 'thread'
        const complete = isThreadComplete(group.messages);
        const agentName = getAgentName(group.messages, getMember);
        return (
          <AgentThread
            key={`thread-${group.workId}-${idx}`}
            messages={group.messages}
            agentName={agentName}
            isComplete={complete}
            getMember={getMember}
          />
        );
      })}
      <div ref={bottomRef} />
    </div>
  );
}
