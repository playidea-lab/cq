import { useState, useEffect, useMemo } from 'react';
import { useProviders } from '../../hooks/useProviders';
import { useSessions } from '../../hooks/useSessions';
import { ProviderTabs } from './ProviderTabs';
import { OverviewPanel } from './OverviewPanel';
import { SessionList } from './SessionList';
import { MessageViewer } from './MessageViewer';
import type { ProviderKind } from '../../types';
import '../../styles/sessions.css';

interface SessionsViewProps {
  projectPath: string;
}

export function SessionsView({ projectPath }: SessionsViewProps) {
  const {
    providers,
    activeProvider,
    setActiveProvider,
    loadProviders,
  } = useProviders();

  const {
    sessions,
    loading,
    error,
    currentSession,
    page,
    messagesLoading,
    listSessions,
    loadMessages,
    loadMore,
  } = useSessions();

  const [searchQuery, setSearchQuery] = useState('');

  // Load providers on mount
  useEffect(() => {
    loadProviders(projectPath);
  }, [projectPath, loadProviders]);

  // Load sessions when provider changes
  useEffect(() => {
    listSessions(projectPath, activeProvider);
  }, [projectPath, activeProvider, listSessions]);

  const handleProviderChange = (kind: ProviderKind) => {
    setActiveProvider(kind);
    setSearchQuery('');
  };

  const filteredSessions = useMemo(() => {
    if (!searchQuery.trim()) return sessions;
    const q = searchQuery.toLowerCase();
    return sessions.filter(s =>
      (s.title && s.title.toLowerCase().includes(q)) ||
      s.id.toLowerCase().includes(q) ||
      (s.git_branch && s.git_branch.toLowerCase().includes(q))
    );
  }, [sessions, searchQuery]);

  return (
    <div className="sessions">
      <div className="sessions__list-panel">
        <ProviderTabs
          providers={providers}
          active={activeProvider}
          onSelect={handleProviderChange}
        />
        <div className="sessions__list-header">
          <h3 className="sessions__list-title">Sessions</h3>
          <span className="sessions__count">{filteredSessions.length}</span>
        </div>
        {sessions.length > 0 && (
          <div className="sessions__search">
            <input
              className="sessions__search-input"
              type="text"
              placeholder="Search sessions..."
              value={searchQuery}
              onChange={e => setSearchQuery(e.target.value)}
            />
          </div>
        )}
        {loading ? (
          <div className="sessions__loading">Loading sessions...</div>
        ) : error ? (
          <div className="sessions__error">{error}</div>
        ) : (
          <SessionList
            sessions={filteredSessions}
            selected={currentSession}
            onSelect={loadMessages}
          />
        )}
      </div>

      <div className="sessions__detail-panel">
        {currentSession ? (
          <>
            <div className="sessions__detail-header">
              <h3 className="sessions__detail-title">
                {currentSession.title || currentSession.id.slice(0, 8)}
              </h3>
              {currentSession.git_branch && (
                <span className="sessions__detail-branch">{currentSession.git_branch}</span>
              )}
              {page && (
                <span className="sessions__detail-lines">{page.total_lines} lines</span>
              )}
            </div>
            <div className="sessions__messages">
              {page ? (
                <MessageViewer
                  messages={page.messages}
                  hasMore={page.has_more}
                  loading={messagesLoading}
                  onLoadMore={loadMore}
                />
              ) : messagesLoading ? (
                <div className="sessions__loading">Loading messages...</div>
              ) : null}
            </div>
          </>
        ) : providers.length > 1 ? (
          <OverviewPanel
            providers={providers}
            onSelectProvider={handleProviderChange}
          />
        ) : (
          <div className="sessions__placeholder">
            Select a session to view its conversation
          </div>
        )}
      </div>
    </div>
  );
}
