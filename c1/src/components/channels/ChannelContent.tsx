import { type ReactNode } from 'react';
import { useMessages } from '../../hooks/useMessages';
import { MessageList } from './MessageList';
import { MessageInput } from './MessageInput';
import type { Channel, C1Member } from '../../types';

interface ChannelContentProps {
  channel: Channel;
  productSlot?: ReactNode;
  isReadOnly?: boolean;
  getMember?: (memberId: string) => C1Member | undefined;
  agentMembers?: C1Member[];
  msgFilter?: string;
}

export function ChannelContent({
  channel,
  productSlot,
  isReadOnly = false,
  getMember,
  agentMembers = [],
  msgFilter = '',
}: ChannelContentProps) {
  const {
    messages,
    loading,
    hasMore,
    loadMore,
    sendMessage,
  } = useMessages(channel.id);

  const filteredMessages = msgFilter.trim()
    ? messages.filter(m =>
        m.content.toLowerCase().includes(msgFilter.toLowerCase())
      )
    : messages;

  const filteredHasMore = !msgFilter.trim() && hasMore;

  return (
    <div className="channel-content">
      {productSlot && (
        <div className="channel-content__product-slot">
          {productSlot}
        </div>
      )}
      <div className="channel-content__conversation">
        <MessageList
          messages={filteredMessages}
          loading={loading}
          hasMore={filteredHasMore}
          onLoadMore={loadMore}
          getMember={getMember}
        />
        {!isReadOnly && (
          <MessageInput
            onSend={(content, metadata) => sendMessage(content, undefined, metadata)}
            agentMembers={agentMembers}
          />
        )}
      </div>
    </div>
  );
}
