import { MarkdownViewer } from '../shared/MarkdownViewer';
import { Skeleton } from '../shared/Skeleton';
import type { KnowledgeDocument } from '../../hooks/useKnowledge';

interface KnowledgeDetailProps {
  doc: KnowledgeDocument | null;
  loading: boolean;
}

export function KnowledgeDetail({ doc, loading }: KnowledgeDetailProps) {
  if (loading) {
    return (
      <div className="knowledge__detail">
        <div className="knowledge__detail-header">
          <Skeleton variant="line" />
        </div>
        <div className="knowledge__detail-body">
          <Skeleton variant="card" count={3} />
        </div>
      </div>
    );
  }

  if (!doc) {
    return (
      <div className="knowledge__detail knowledge__detail--empty">
        Select a document to view
      </div>
    );
  }

  const formatDate = (iso: string) => {
    try {
      return new Date(iso).toLocaleDateString(undefined, {
        year: 'numeric',
        month: 'short',
        day: 'numeric',
      });
    } catch {
      return iso;
    }
  };

  return (
    <div className="knowledge__detail">
      <div className="knowledge__detail-header">
        <div>
          <h3 className="knowledge__detail-title">{doc.title}</h3>
          <div className="knowledge__detail-meta">
            <span className="knowledge__detail-type">{doc.doc_type}</span>
            {doc.domain && (
              <span className="knowledge__detail-domain">{doc.domain}</span>
            )}
            <span className="knowledge__detail-date">
              Updated {formatDate(doc.updated_at)}
            </span>
            <span className="knowledge__detail-version">v{doc.version}</span>
          </div>
        </div>
        {doc.tags.length > 0 && (
          <div className="knowledge__detail-tags">
            {doc.tags.map(tag => (
              <span key={tag} className="knowledge__tag">{tag}</span>
            ))}
          </div>
        )}
      </div>
      <div className="knowledge__detail-body">
        <MarkdownViewer content={doc.body} />
      </div>
    </div>
  );
}
