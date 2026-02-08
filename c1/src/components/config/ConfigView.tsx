import { useEffect } from 'react';
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
  } = useConfig();

  useEffect(() => {
    loadFiles(projectPath);
  }, [projectPath, loadFiles]);

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
              {selectedFile.truncated && (
                <span className="badge badge--orange">Truncated</span>
              )}
            </div>
            <div className="config__content-body">
              {selectedFile.path.endsWith('.md') ? (
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
