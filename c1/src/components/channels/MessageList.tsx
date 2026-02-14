import { useEffect, useRef } from 'react';
import { MessageBubble } from './MessageBubble';
import { Skeleton } from '../shared/Skeleton';
import type { C1Message } from '../../types';

interface MessageListProps {
  messages: C1Message[];
  loading: boolean;
  hasMore: boolean;
  onLoadMore: () => void;
}

export function MessageList({ messages, loading, hasMore, onLoadMore }: MessageListProps) {
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
      {messages.map(msg => (
        <MessageBubble key={msg.id} message={msg} />
      ))}
      <div ref={bottomRef} />
    </div>
  );
}
