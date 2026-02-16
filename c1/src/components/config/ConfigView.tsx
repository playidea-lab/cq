import { useEffect, useState } from 'react';
import { useConfig } from '../../hooks/useConfig';
import { MarkdownViewer } from '../shared/MarkdownViewer';
import { Skeleton } from '../shared/Skeleton';
import { ErrorState } from '../shared/ErrorState';
import { formatSize } from '../../utils/format';
import '../../styles/config.css';

interface ConfigViewProps {
  projectPath: string;
}

export function ConfigView({ projectPath }: ConfigViewProps) {
  const {
    grouped,
    selectedFile,
    loading,
    contentLoading,
    error,
    loadFiles,
    loadContent,
    saveConfig,
  } = useConfig();

  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState('');
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    loadFiles(projectPath);
  }, [projectPath, loadFiles]);

  // Reset edit mode when selected file changes
  useEffect(() => {
    setEditing(false);
    if (selectedFile) setDraft(selectedFile.content);
  }, [selectedFile]);

  const handleToggleEdit = () => {
    if (!editing && selectedFile) {
      setDraft(selectedFile.content);
    }
    setEditing(!editing);
  };

  const handleSave = async () => {
    if (!selectedFile) return;
    setSaving(true);
    try {
      await saveConfig(selectedFile.path, draft);
      setEditing(false);
    } catch {
      // error is set in useConfig
    } finally {
      setSaving(false);
    }
  };

  const hasChanges = editing && selectedFile && draft !== selectedFile.content;

  if (loading) {
    return (
      <div className="config">
        <div className="config__tree-panel">
          <Skeleton variant="list-item" count={8} />
        </div>
        <div className="config__content-panel">
          <Skeleton variant="card" count={1} />
        </div>
      </div>
    );
  }

  if (error) {
    return (
      <ErrorState
        message="Failed to load config files"
        detail={error}
        onRetry={() => loadFiles(projectPath)}
      />
    );
  }

  return (
    <div className="config">
      <div className="config__tree-panel">
        <h3 className="config__tree-title">Configuration</h3>
        {Object.entries(grouped).map(([category, { label, files }]) => (
          <div key={category} className="config__category">
            <div className="config__category-header">
              {label} ({files.length})
            </div>
            <ul className="config__file-list">
              {files.map(file => (
                <li key={file.path}>
                  <button
                    className={`config__file-item ${selectedFile?.path === file.path ? 'config__file-item--active' : ''}`}
                    onClick={() => loadContent(file.path)}
                    title={file.path}
                  >
                    <span className="config__file-name">{file.name}</span>
                    <span className="config__file-size">{formatSize(file.size)}</span>
                  </button>
                </li>
              ))}
            </ul>
          </div>
        ))}
      </div>

      <div className="config__content-panel">
        {contentLoading ? (
          <Skeleton variant="card" count={1} />
        ) : selectedFile ? (
          <>
            <div className="config__content-header">
              <span className="config__content-path">{selectedFile.path}</span>
              <div className="config__content-actions">
                {selectedFile.truncated && (
                  <span className="badge badge--orange">Truncated</span>
                )}
                <button
                  className={`config__edit-btn ${editing ? 'config__edit-btn--active' : ''}`}
                  onClick={handleToggleEdit}
                >
                  {editing ? 'Preview' : 'Edit'}
                </button>
                {editing && (
                  <button
                    className="config__save-btn"
                    onClick={handleSave}
                    disabled={saving || !hasChanges}
                  >
                    {saving ? 'Saving...' : 'Save'}
                  </button>
                )}
              </div>
            </div>
            <div className="config__content-body">
              {editing ? (
                <textarea
                  className="config__textarea"
                  value={draft}
                  onChange={e => setDraft(e.target.value)}
                />
              ) : selectedFile.path.endsWith('.md') ? (
                <MarkdownViewer content={selectedFile.content} />
              ) : (
                <pre className="config__content-raw">{selectedFile.content}</pre>
              )}
            </div>
          </>
        ) : (
          <div className="config__placeholder">
            Select a file to view its content
          </div>
        )}
      </div>
    </div>
  );
}
