import { useState, useCallback } from 'react';
import { useKnowledge } from '../../hooks/useKnowledge';
import { KnowledgeDetail } from './KnowledgeDetail';
import { Skeleton } from '../shared/Skeleton';
import { ErrorState } from '../shared/ErrorState';
import '../../styles/knowledge.css';

interface KnowledgeViewProps {
  projectPath: string;
}

const DOC_TYPES = ['all', 'experiment', 'pattern', 'insight', 'hypothesis'] as const;

export function KnowledgeView({ projectPath }: KnowledgeViewProps) {
  const {
    items,
    selectedDoc,
    stats,
    loading,
    docLoading,
    error,
    search,
    loadDoc,
  } = useKnowledge(projectPath);

  const [query, setQuery] = useState('');
  const [activeType, setActiveType] = useState<string>('all');

  const handleSearch = useCallback(() => {
    search(
      query.trim() || undefined,
      activeType === 'all' ? undefined : activeType,
    );
  }, [query, activeType, search]);

  const handleTypeChange = (type: string) => {
    setActiveType(type);
    search(
      query.trim() || undefined,
      type === 'all' ? undefined : type,
    );
  };

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter') handleSearch();
  };

  const handleSelectItem = (id: string) => {
    loadDoc(id);
  };

  if (error && items.length === 0) {
    return (
      <ErrorState
        message="Failed to load knowledge"
        detail={error}
        onRetry={() => search()}
      />
    );
  }

  return (
    <div className="knowledge">
      <aside className="knowledge__sidebar">
        <div className="knowledge__header">
          <div className="knowledge__title-row">
            <span className="knowledge__title">Knowledge</span>
            {stats && (
              <span className="knowledge__count">{stats.total_documents}</span>
            )}
          </div>
          <div className="knowledge__search">
            <input
              className="knowledge__search-input"
              type="text"
              placeholder="Search documents..."
              value={query}
              onChange={e => setQuery(e.target.value)}
              onKeyDown={handleKeyDown}
            />
          </div>
          <div className="knowledge__filters">
            {DOC_TYPES.map(type => (
              <button
                key={type}
                className={`knowledge__filter ${activeType === type ? 'knowledge__filter--active' : ''}`}
                onClick={() => handleTypeChange(type)}
              >
                {type === 'all' ? 'All' : type}
              </button>
            ))}
          </div>
        </div>
        <ul className="knowledge__list">
          {loading ? (
            <li style={{ padding: '8px 16px' }}>
              <Skeleton variant="list-item" count={5} />
            </li>
          ) : items.length === 0 ? (
            <li className="knowledge__empty-item">
              No documents found
            </li>
          ) : (
            items.map(item => (
              <li
                key={item.id}
                className={`knowledge__item ${selectedDoc?.id === item.id ? 'knowledge__item--active' : ''}`}
                onClick={() => handleSelectItem(item.id)}
              >
                <div className="knowledge__item-header">
                  <span className="knowledge__item-type">{item.doc_type}</span>
                  <span className="knowledge__item-domain">{item.domain}</span>
                </div>
                <span className="knowledge__item-title">{item.title}</span>
                {item.tags.length > 0 && (
                  <div className="knowledge__item-tags">
                    {item.tags.slice(0, 3).map(tag => (
                      <span key={tag} className="knowledge__tag">{tag}</span>
                    ))}
                  </div>
                )}
              </li>
            ))
          )}
        </ul>
      </aside>
      <KnowledgeDetail doc={selectedDoc} loading={docLoading} />
    </div>
  );
}
