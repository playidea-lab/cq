import { useState, useEffect } from 'react';
import { invoke } from '@tauri-apps/api/core';
import { useChannels } from '../../hooks/useChannels';
import { useMessages } from '../../hooks/useMessages';
import { useMembers } from '../../hooks/useMembers';
import { usePresence } from '../../hooks/usePresence';
import { useSessions } from '../../hooks/useSessions';
import { ChannelSidebar } from './ChannelSidebar';
import { MessageList } from './MessageList';
import { MessageInput } from './MessageInput';
import { MembersPanel } from './MembersPanel';
import { SessionList } from '../sessions/SessionList';
import { MessageViewer } from '../sessions/MessageViewer';
import type { ProviderKind } from '../../types';
import '../../styles/channels.css';
import '../../styles/sessions.css';

type MessengerTab = 'channels' | 'sessions';

const PROVIDERS: { kind: ProviderKind; label: string }[] = [
  { kind: 'claude_code', label: 'Claude' },
  { kind: 'codex_cli', label: 'Codex' },
  { kind: 'cursor', label: 'Cursor' },
  { kind: 'gemini_cli', label: 'Gemini' },
];

interface ChannelsViewProps {
  projectPath: string;
}

export function ChannelsView({ projectPath }: ChannelsViewProps) {
  const [activeTab, setActiveTab] = useState<MessengerTab>('sessions');
  const [showMembers, setShowMembers] = useState(false);
  const [projectId, setProjectId] = useState<string | null>(null);

  // Resolve projectPath → actual project_id via Rust IPC
  useEffect(() => {
    let cancelled = false;
    invoke<string>('get_project_id_cmd', { path: projectPath })
      .then((id) => {
        if (!cancelled) setProjectId(id);
      })
      .catch(() => {
        // Fallback: use directory name
        if (!cancelled) {
          const parts = projectPath.replace(/\\/g, '/').split('/');
          setProjectId(parts[parts.length - 1] || projectPath);
        }
      });
    return () => { cancelled = true; };
  }, [projectPath]);

  // --- Channels hooks ---
  const {
    channels,
    loading: channelsLoading,
    selectedChannel,
    createChannel,
    selectChannel,
  } = useChannels(projectId);

  const {
    messages: channelMessages,
    loading: messagesLoading,
    hasMore: channelHasMore,
    loadMore: channelLoadMore,
    sendMessage,
  } = useMessages(selectedChannel?.id ?? null);

  const { members, getMember } = useMembers(projectId);
  usePresence(projectId);

  // --- Sessions hooks ---
  const {
    sessions,
    loading: sessionsLoading,
    currentSession,
    page,
    messagesLoading: sessionMessagesLoading,
    currentProvider,
    listSessions,
    loadMessages,
    loadMore: sessionLoadMore,
    startWatching,
  } = useSessions();

  // Load sessions when tab becomes active or provider changes
  useEffect(() => {
    if (activeTab === 'sessions' && projectPath) {
      listSessions(projectPath, currentProvider);
      startWatching(projectPath);
    }
  }, [activeTab, projectPath, currentProvider, listSessions, startWatching]);

  const handleProviderChange = (provider: ProviderKind) => {
    listSessions(projectPath, provider);
  };

  return (
    <div className="channels">
      {/* Tab bar */}
      <div className="messenger-tabs">
        <button
          className={`messenger-tabs__btn ${activeTab === 'sessions' ? 'messenger-tabs__btn--active' : ''}`}
          onClick={() => setActiveTab('sessions')}
        >
          Sessions
        </button>
        <button
          className={`messenger-tabs__btn ${activeTab === 'channels' ? 'messenger-tabs__btn--active' : ''}`}
          onClick={() => setActiveTab('channels')}
        >
          Channels
        </button>
      </div>

      {activeTab === 'channels' ? (
        /* --- Cloud Channels view --- */
        <>
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
                    messages={channelMessages}
                    loading={messagesLoading}
                    hasMore={channelHasMore}
                    onLoadMore={channelLoadMore}
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
        </>
      ) : (
        /* --- Sessions view (local conversations) --- */
        <>
          <div className="session-sidebar">
            <div className="session-sidebar__header">
              <div className="session-sidebar__providers">
                {PROVIDERS.map(p => (
                  <button
                    key={p.kind}
                    className={`session-sidebar__provider ${currentProvider === p.kind ? 'session-sidebar__provider--active' : ''}`}
                    onClick={() => handleProviderChange(p.kind)}
                  >
                    {p.label}
                  </button>
                ))}
              </div>
            </div>
            <SessionList
              sessions={sessions}
              selected={currentSession}
              onSelect={loadMessages}
            />
            {sessionsLoading && (
              <div className="session-sidebar__loading">Loading...</div>
            )}
          </div>
          <div className="chat-panel">
            {currentSession && page ? (
              <>
                <div className="chat-panel__header">
                  <span className="chat-panel__channel-name">
                    {currentSession.title || currentSession.id.slice(0, 12)}
                  </span>
                  <span className="chat-panel__channel-desc">
                    {currentProvider}
                    {currentSession.git_branch && ` · ${currentSession.git_branch}`}
                  </span>
                </div>
                <MessageViewer
                  messages={page.messages}
                  hasMore={page.has_more}
                  loading={sessionMessagesLoading}
                  onLoadMore={sessionLoadMore}
                />
              </>
            ) : (
              <div className="chat-panel__empty">
                Select a session to view conversation
              </div>
            )}
          </div>
        </>
      )}
    </div>
  );
}
