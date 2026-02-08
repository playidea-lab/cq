import { useState, useEffect, useMemo, useRef, useCallback } from 'react';
import { useProviders } from '../../hooks/useProviders';
import { useSessions } from '../../hooks/useSessions';
import { useEditors } from '../../hooks/useEditors';
import { ProviderTabs } from './ProviderTabs';
import { OverviewPanel } from './OverviewPanel';
import { SessionList } from './SessionList';
import { SearchResults } from './SearchResults';
import { MessageViewer } from './MessageViewer';
import { AnalyticsPanel } from './AnalyticsPanel';
import { FilterBar } from './FilterBar';
import { Skeleton } from '../shared/Skeleton';
import { ErrorState } from '../shared/ErrorState';
import type { ProviderKind, SearchHit, SessionMeta } from '../../types';
import type { SortKey, TimeFilter } from './FilterBar';
import '../../styles/sessions.css';

interface SessionsViewProps {
  projectPath: string;
}

function applyTimeFilter(sessions: SessionMeta[], filter: TimeFilter): SessionMeta[] {
  if (filter === 'all') return sessions;
  const now = Date.now();
  const cutoff = filter === 'today' ? 86400000 : filter === 'week' ? 604800000 : 2592000000;
  return sessions.filter(s => s.timestamp != null && now - s.timestamp < cutoff);
}

function applySort(sessions: SessionMeta[], key: SortKey): SessionMeta[] {
  const sorted = [...sessions];
  switch (key) {
    case 'date':
      return sorted.sort((a, b) => (b.timestamp ?? 0) - (a.timestamp ?? 0));
    case 'size':
      return sorted.sort((a, b) => b.file_size - a.file_size);
    case 'name':
      return sorted.sort((a, b) => (a.title || a.id).localeCompare(b.title || b.id));
    default:
      return sorted;
  }
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
  const [detailTab, setDetailTab] = useState<'messages' | 'analytics'>('messages');
  const [sortBy, setSortBy] = useState<SortKey>('date');
  const [timeFilter, setTimeFilter] = useState<TimeFilter>('all');
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(() => {
    loadProviders(projectPath);
  }, [projectPath, loadProviders]);

  useEffect(() => {
    listSessions(projectPath, activeProvider);
  }, [projectPath, activeProvider, listSessions]);

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
    const session = sessions.find(s => s.id === hit.session_id);
    if (session) {
      loadMessages(session);
    } else {
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
    let result = sessions;

    // Text filter
    if (searchQuery.trim()) {
      const q = searchQuery.toLowerCase();
      result = result.filter(s =>
        (s.title && s.title.toLowerCase().includes(q)) ||
        s.id.toLowerCase().includes(q) ||
        (s.git_branch && s.git_branch.toLowerCase().includes(q))
      );
    }

    // Time filter
    result = applyTimeFilter(result, timeFilter);

    // Sort
    result = applySort(result, sortBy);

    return result;
  }, [sessions, searchQuery, timeFilter, sortBy]);

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
          <>
            <div className="sessions__search">
              <input
                className="sessions__search-input"
                type="text"
                placeholder="Search sessions..."
                value={searchQuery}
                onChange={e => setSearchQuery(e.target.value)}
              />
            </div>
            <FilterBar
              sortBy={sortBy}
              onSortChange={setSortBy}
              timeFilter={timeFilter}
              onTimeFilterChange={setTimeFilter}
            />
          </>
        )}
        {loading ? (
          <div className="sessions__loading">
            <Skeleton variant="list-item" count={6} />
          </div>
        ) : error ? (
          <ErrorState
            message="Failed to load sessions"
            detail={error}
            onRetry={() => listSessions(projectPath, activeProvider)}
          />
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
              {page && detailTab === 'messages' && (
                <span className="sessions__detail-lines">{page.total_lines} lines</span>
              )}
              <div className="sessions__detail-tabs">
                <button
                  className={`sessions__detail-tab${detailTab === 'messages' ? ' sessions__detail-tab--active' : ''}`}
                  onClick={() => setDetailTab('messages')}
                >
                  Messages
                </button>
                <button
                  className={`sessions__detail-tab${detailTab === 'analytics' ? ' sessions__detail-tab--active' : ''}`}
                  onClick={() => setDetailTab('analytics')}
                >
                  Analytics
                </button>
              </div>
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
              {detailTab === 'analytics' ? (
                <AnalyticsPanel
                  sessionPath={currentSession.path}
                  provider={activeProvider}
                />
              ) : page ? (
                <MessageViewer
                  messages={page.messages}
                  hasMore={page.has_more}
                  loading={messagesLoading}
                  onLoadMore={loadMore}
                />
              ) : messagesLoading ? (
                <div className="sessions__loading">
                  <Skeleton variant="card" count={3} />
                </div>
              ) : null}
            </div>
          </>
        ) : providers.length > 1 ? (
          <OverviewPanel
            providers={providers}
            onSelectProvider={handleProviderChange}
            projectPath={projectPath}
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
