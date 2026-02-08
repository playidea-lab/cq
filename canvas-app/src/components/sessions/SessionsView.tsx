import { useState, useEffect, useMemo, useRef, useCallback } from 'react';
import { useProviders } from '../../hooks/useProviders';
import { useSessions } from '../../hooks/useSessions';
import { useEditors } from '../../hooks/useEditors';
import { ProviderTabs } from './ProviderTabs';
import { OverviewPanel } from './OverviewPanel';
import { SessionList } from './SessionList';
import { SearchResults } from './SearchResults';
import { MessageViewer } from './MessageViewer';
import type { ProviderKind, SearchHit } from '../../types';
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
    searchResults,
    searchLoading,
    listSessions,
    loadMessages,
    loadMore,
    searchContent,
    clearSearchResults,
    startWatching,
  } = useSessions();

  const { editors, openInEditor, getLabel } = useEditors();

  const [searchQuery, setSearchQuery] = useState('');
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  // Load providers on mount
  useEffect(() => {
    loadProviders(projectPath);
  }, [projectPath, loadProviders]);

  // Load sessions when provider changes
  useEffect(() => {
    listSessions(projectPath, activeProvider);
  }, [projectPath, activeProvider, listSessions]);

  // Start file watcher for auto-refresh
  useEffect(() => {
    startWatching(projectPath);
  }, [projectPath, startWatching]);

  // Debounced content search
  useEffect(() => {
    if (debounceRef.current) {
      clearTimeout(debounceRef.current);
    }

    const trimmed = searchQuery.trim();
    if (trimmed.length <= 2) {
      clearSearchResults();
      return;
    }

    debounceRef.current = setTimeout(() => {
      searchContent(projectPath, trimmed);
    }, 300);

    return () => {
      if (debounceRef.current) {
        clearTimeout(debounceRef.current);
      }
    };
  }, [searchQuery, projectPath, searchContent, clearSearchResults]);

  const handleProviderChange = (kind: ProviderKind) => {
    setActiveProvider(kind);
    setSearchQuery('');
    clearSearchResults();
  };

  const handleSearchHitClick = useCallback((hit: SearchHit) => {
    // Find the session in the loaded sessions list and load it
    const session = sessions.find(s => s.id === hit.session_id);
    if (session) {
      loadMessages(session);
    } else {
      // Create a minimal session meta to load messages directly
      loadMessages({
        id: hit.session_id,
        slug: '',
        title: hit.session_title,
        path: hit.session_path,
        line_count: 0,
        file_size: 0,
        timestamp: null,
        git_branch: null,
      });
    }
  }, [sessions, loadMessages]);

  const filteredSessions = useMemo(() => {
    if (!searchQuery.trim()) return sessions;
    const q = searchQuery.toLowerCase();
    return sessions.filter(s =>
      (s.title && s.title.toLowerCase().includes(q)) ||
      s.id.toLowerCase().includes(q) ||
      (s.git_branch && s.git_branch.toLowerCase().includes(q))
    );
  }, [sessions, searchQuery]);

  // Show content search results when query is long enough and results exist
  const showSearchResults = searchQuery.trim().length > 2 && searchResults !== null;

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
        ) : showSearchResults ? (
          <SearchResults
            results={searchResults!}
            loading={searchLoading}
            onSelect={handleSearchHitClick}
          />
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
              {editors.length > 0 && (
                <button
                  className="btn btn--sm btn--secondary sessions__open-editor"
                  onClick={() => openInEditor(projectPath)}
                  title={`Open project in ${getLabel(editors[0])}`}
                >
                  Open in {getLabel(editors[0])}
                </button>
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
