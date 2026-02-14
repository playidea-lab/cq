import { useState, useEffect } from 'react';
import { MarkdownViewer } from '../shared/MarkdownViewer';
import { Skeleton } from '../shared/Skeleton';
import { useDocument } from '../../hooks/useDocuments';

interface DocumentEditorProps {
  path: string | null;
}

function formatSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

export function DocumentEditor({ path }: DocumentEditorProps) {
  const { doc, loading, saving, save } = useDocument(path);
  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState('');

  // Reset edit mode when document changes
  useEffect(() => {
    setEditing(false);
    if (doc) setDraft(doc.content);
  }, [doc]);

  if (!path) {
    return <div className="doc-editor__empty">Select a document to view</div>;
  }

  if (loading) {
    return (
      <div className="doc-editor">
        <div className="doc-editor__header">
          <Skeleton variant="line" />
        </div>
        <div className="doc-editor__content">
          <Skeleton variant="card" count={3} />
        </div>
      </div>
    );
  }

  if (!doc) {
    return <div className="doc-editor__empty">Document not found</div>;
  }

  const handleToggleEdit = () => {
    if (!editing) {
      setDraft(doc.content);
    }
    setEditing(!editing);
  };

  const handleSave = async () => {
    await save(draft);
    setEditing(false);
  };

  const hasChanges = editing && draft !== doc.content;

  return (
    <div className="doc-editor">
      <div className="doc-editor__header">
        <div>
          <span className="doc-editor__name">{doc.name}</span>
          <span className="doc-editor__meta">
            {doc.doc_type} &middot; {formatSize(doc.content.length)}
          </span>
        </div>
        <div className="doc-editor__actions">
          <button
            className={`doc-editor__toggle-btn ${editing ? 'doc-editor__toggle-btn--active' : ''}`}
            onClick={handleToggleEdit}
          >
            {editing ? 'Preview' : 'Edit'}
          </button>
          {editing && (
            <button
              className="doc-editor__save-btn"
              onClick={handleSave}
              disabled={saving || !hasChanges}
            >
              {saving ? 'Saving...' : 'Save'}
            </button>
          )}
        </div>
      </div>
      <div className="doc-editor__content">
        {editing ? (
          <textarea
            className="doc-editor__textarea"
            value={draft}
            onChange={e => setDraft(e.target.value)}
          />
        ) : (
          <MarkdownViewer content={doc.content} />
        )}
      </div>
    </div>
  );
}
