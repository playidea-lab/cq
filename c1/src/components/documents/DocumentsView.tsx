import { useState } from 'react';
import { useDocuments } from '../../hooks/useDocuments';
import { DocumentEditor } from './DocumentEditor';
import { CreateDocumentDialog } from './CreateDocumentDialog';
import { Skeleton } from '../shared/Skeleton';
import type { DocType, DocumentMeta } from '../../types';
import '../../styles/documents.css';

interface DocumentsViewProps {
  projectPath: string;
}

const DOC_TABS: { type: DocType; label: string }[] = [
  { type: 'persona', label: 'Personas' },
  { type: 'skill', label: 'Skills' },
  { type: 'spec', label: 'Specs' },
  { type: 'config', label: 'Config' },
];

function formatSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  return `${(bytes / 1024).toFixed(1)} KB`;
}

export function DocumentsView({ projectPath }: DocumentsViewProps) {
  const [activeTab, setActiveTab] = useState<DocType>('persona');
  const [selectedDoc, setSelectedDoc] = useState<DocumentMeta | null>(null);
  const [showCreate, setShowCreate] = useState(false);
  const [docSearch, setDocSearch] = useState('');
  const { documents, loading, createDocument, deleteDocument } = useDocuments(projectPath, activeTab);

  const filteredDocs = docSearch.trim()
    ? documents.filter(d => d.name.toLowerCase().includes(docSearch.toLowerCase()))
    : documents;

  const handleTabChange = (type: DocType) => {
    setActiveTab(type);
    setSelectedDoc(null);
  };

  const handleCreate = async (name: string, content: string) => {
    await createDocument(name, content);
    setShowCreate(false);
  };

  const handleDelete = async (path: string) => {
    await deleteDocument(path);
    setSelectedDoc(null);
  };

  return (
    <div className="documents">
      <aside className="doc-sidebar">
        <div className="doc-sidebar__header">
          <div className="doc-sidebar__title-row">
            <span className="doc-sidebar__title">Documents</span>
            <button
              className="btn btn--primary btn--sm"
              onClick={() => setShowCreate(true)}
              title={`New ${activeTab}`}
            >
              + New
            </button>
          </div>
          <div className="doc-sidebar__tabs">
            {DOC_TABS.map(tab => (
              <button
                key={tab.type}
                className={`doc-sidebar__tab ${activeTab === tab.type ? 'doc-sidebar__tab--active' : ''}`}
                onClick={() => handleTabChange(tab.type)}
              >
                {tab.label}
              </button>
            ))}
          </div>
          <input
            className="search-input"
            type="text"
            placeholder="Search documents..."
            value={docSearch}
            onChange={e => setDocSearch(e.target.value)}
          />
        </div>
        <ul className="doc-sidebar__list">
          {loading ? (
            <li style={{ padding: '8px 16px' }}>
              <Skeleton variant="list-item" count={3} />
            </li>
          ) : filteredDocs.length === 0 ? (
            <li style={{ padding: '8px 16px', color: 'var(--color-text-muted)', fontSize: 'var(--font-size-sm)' }}>
              {docSearch.trim() ? 'No matches' : `No ${activeTab} documents found`}
            </li>
          ) : (
            filteredDocs.map(doc => (
              <li
                key={doc.path}
                className={`doc-sidebar__item ${selectedDoc?.path === doc.path ? 'doc-sidebar__item--active' : ''}`}
                onClick={() => setSelectedDoc(doc)}
              >
                <span className="doc-sidebar__item-name">{doc.name}</span>
                <span className="doc-sidebar__item-size">{formatSize(doc.size)}</span>
              </li>
            ))
          )}
        </ul>
      </aside>
      <DocumentEditor
        path={selectedDoc?.path ?? null}
        onDelete={handleDelete}
      />
      {showCreate && (
        <CreateDocumentDialog
          docType={activeTab}
          onConfirm={handleCreate}
          onCancel={() => setShowCreate(false)}
        />
      )}
    </div>
  );
}
