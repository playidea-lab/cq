import { useChannels } from '../../hooks/useChannels';
import { useMessages } from '../../hooks/useMessages';
import { ChannelSidebar } from './ChannelSidebar';
import { MessageList } from './MessageList';
import { MessageInput } from './MessageInput';
import '../../styles/channels.css';

interface ChannelsViewProps {
  projectId: string;
}

export function ChannelsView({ projectId }: ChannelsViewProps) {
  const {
    channels,
    loading: channelsLoading,
    selectedChannel,
    createChannel,
    selectChannel,
  } = useChannels(projectId);

  const {
    messages,
    loading: messagesLoading,
    hasMore,
    loadMore,
    sendMessage,
  } = useMessages(selectedChannel?.id ?? null);

  return (
    <div className="channels">
      <ChannelSidebar
        channels={channels}
        selectedChannel={selectedChannel}
        loading={channelsLoading}
        onSelect={selectChannel}
        onCreate={createChannel}
      />
      <div className="chat-panel">
        {selectedChannel ? (
          <>
            <div className="chat-panel__header">
              <span className="chat-panel__channel-name">#{selectedChannel.name}</span>
              {selectedChannel.description && (
                <span className="chat-panel__channel-desc">{selectedChannel.description}</span>
              )}
            </div>
            <MessageList
              messages={messages}
              loading={messagesLoading}
              hasMore={hasMore}
              onLoadMore={loadMore}
            />
            <MessageInput onSend={content => sendMessage(content)} />
          </>
        ) : (
          <div className="chat-panel__empty">
            Select a channel to start messaging
          </div>
        )}
      </div>
    </div>
  );
}
