import { useState, useEffect, useCallback } from 'react';
import { invoke } from '@tauri-apps/api/core';
import { useChannels } from '../../hooks/useChannels';
import { useMembers } from '../../hooks/useMembers';
import { usePresence } from '../../hooks/usePresence';
import { useSessions } from '../../hooks/useSessions';
import { ChannelListSidebar } from './ChannelListSidebar';
import { ChannelContent } from './ChannelContent';
import { MembersPanel } from './MembersPanel';
import { ProductView } from './ProductView';
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
];

interface ChannelsViewProps {
  projectPath: string;
}

export function ChannelsView({ projectPath }: ChannelsViewProps) {
  const [activeTab, setActiveTab] = useState<MessengerTab>('sessions');
  const [showMembers, setShowMembers] = useState(false);
  const [channelMsgFilter, setChannelMsgFilter] = useState('');
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
    selectedChannel,
    createChannel,
    selectChannel,
  } = useChannels(projectId);

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
    searchResults,
    searchLoading,
    listSessions,
    loadMessages,
    loadMore: sessionLoadMore,
    searchContent,
    clearSearchResults,
    startWatching,
  } = useSessions();

  const [sessionSearchQuery, setSessionSearchQuery] = useState('');
  const [msgFilter, setMsgFilter] = useState('');

  const handleSessionSearch = useCallback((q: string) => {
    setSessionSearchQuery(q);
    if (q.trim().length >= 2) {
      searchContent(projectPath, q.trim());
    } else {
      clearSearchResults();
    }
  }, [projectPath, searchContent, clearSearchResults]);

  // Sync session channels on mount (best-effort, non-blocking)
  useEffect(() => {
    if (projectId && projectPath) {
      invoke('sync_session_channels', { projectId, projectPath }).catch(console.error);
    }
  }, [projectId, projectPath]);

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
          <ChannelListSidebar
            channels={channels}
            selectedChannel={selectedChannel}
            onSelect={selectChannel}
            onCreate={(name, type) => { createChannel(name, '', type); }}
          />
          <div className="chat-panel">
            {selectedChannel ? (
              <>
                <div className="chat-panel__header">
                  <span className="chat-panel__channel-name">#{selectedChannel.name}</span>
                  {selectedChannel.description && (
                    <span className="chat-panel__channel-desc">{selectedChannel.description}</span>
                  )}
                  <input
                    className="search-input search-input--inline"
                    type="text"
                    placeholder="Filter messages..."
                    value={channelMsgFilter}
                    onChange={e => setChannelMsgFilter(e.target.value)}
                  />
                  <button
                    className="chat-panel__members-toggle"
                    onClick={() => setShowMembers(!showMembers)}
                    title="Toggle members panel"
                  >
                    {members.length} members
                  </button>
                </div>
                <div className="chat-panel__body">
                  <ChannelContent
                    channel={selectedChannel}
                    productSlot={<ProductView channelId={selectedChannel.id} />}
                    getMember={getMember}
                    agentMembers={members.filter(m => m.member_type === 'agent')}
                    msgFilter={channelMsgFilter}
                  />
                  {showMembers && (
                    <MembersPanel members={members} />
                  )}
                </div>
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
              <input
                className="search-input"
                type="text"
                placeholder="Search sessions..."
                value={sessionSearchQuery}
                onChange={e => handleSessionSearch(e.target.value)}
              />
            </div>
            {searchResults !== null ? (
              <div className="search-results">
                {searchLoading && <div className="search-results__loading">Searching...</div>}
                {!searchLoading && searchResults.length === 0 && (
                  <div className="search-results__empty">No matches</div>
                )}
                {searchResults.map((hit, i) => (
                  <button
                    key={`${hit.session_id}-${hit.line_number}-${i}`}
                    className="search-results__item"
                    onClick={() => {
                      const session = sessions.find(s => s.id === hit.session_id);
                      if (session) loadMessages(session);
                    }}
                  >
                    <div className="search-results__title">
                      {hit.session_title || hit.session_id.slice(0, 8)}
                    </div>
                    <div className="search-results__context">{hit.matched_text}</div>
                  </button>
                ))}
              </div>
            ) : (
              <SessionList
                sessions={sessions}
                selected={currentSession}
                onSelect={loadMessages}
              />
            )}
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
                  <input
                    className="search-input search-input--inline"
                    type="text"
                    placeholder="Filter messages..."
                    value={msgFilter}
                    onChange={e => setMsgFilter(e.target.value)}
                  />
                </div>
                <MessageViewer
                  key={currentSession.id}
                  messages={msgFilter.trim()
                    ? page.messages.filter(m =>
                        m.content.some(b => b.text?.toLowerCase().includes(msgFilter.toLowerCase()))
                      )
                    : page.messages}
                  hasMore={!msgFilter.trim() && page.has_more}
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
