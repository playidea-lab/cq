import { useEffect } from 'react';
import { useSessions } from '../../hooks/useSessions';
import { SessionList } from './SessionList';
import { MessageViewer } from './MessageViewer';
import '../../styles/sessions.css';

interface SessionsViewProps {
  projectPath: string;
}

export function SessionsView({ projectPath }: SessionsViewProps) {
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

  useEffect(() => {
    listSessions(projectPath);
  }, [projectPath, listSessions]);

  return (
    <div className="sessions">
      <div className="sessions__list-panel">
        <div className="sessions__list-header">
          <h3 className="sessions__list-title">Sessions</h3>
          <span className="sessions__count">{sessions.length}</span>
        </div>
        {loading ? (
          <div className="sessions__loading">Loading sessions...</div>
        ) : error ? (
          <div className="sessions__error">{error}</div>
        ) : (
          <SessionList
            sessions={sessions}
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
        ) : (
          <div className="sessions__placeholder">
            Select a session to view its conversation
          </div>
        )}
      </div>
    </div>
  );
}
