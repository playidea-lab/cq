import { useState } from 'react';
import { useChannels } from '../../hooks/useChannels';
import { useMessages } from '../../hooks/useMessages';
import { useMembers } from '../../hooks/useMembers';
import { usePresence } from '../../hooks/usePresence';
import { ChannelSidebar } from './ChannelSidebar';
import { MessageList } from './MessageList';
import { MessageInput } from './MessageInput';
import { MembersPanel } from './MembersPanel';
import '../../styles/channels.css';

interface ChannelsViewProps {
  projectId: string;
}

export function ChannelsView({ projectId }: ChannelsViewProps) {
  const [showMembers, setShowMembers] = useState(false);

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

  const { members, getMember } = useMembers(projectId);
  usePresence(projectId);

  return (
    <div className="channels">
      <ChannelSidebar
        channels={channels}
        selectedChannel={selectedChannel}
        loading={channelsLoading}
        onSelect={selectChannel}
        onCreate={createChannel}
        members={members}
      />
      <div className="chat-panel">
        {selectedChannel ? (
          <>
            <div className="chat-panel__header">
              <span className="chat-panel__channel-name">#{selectedChannel.name}</span>
              {selectedChannel.description && (
                <span className="chat-panel__channel-desc">{selectedChannel.description}</span>
              )}
              <button
                className="chat-panel__members-toggle"
                onClick={() => setShowMembers(!showMembers)}
                title="Toggle members panel"
              >
                {members.length} members
              </button>
            </div>
            <div className="chat-panel__body">
              <MessageList
                messages={messages}
                loading={messagesLoading}
                hasMore={hasMore}
                onLoadMore={loadMore}
                getMember={getMember}
              />
              {showMembers && (
                <MembersPanel members={members} />
              )}
            </div>
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
