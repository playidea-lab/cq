import '../styles/nodes.css';

interface ToolbarProps {
  onRefresh: () => void;
  onOpenFolder: () => void;
  loading: boolean;
  projectPath?: string;
}

export function Toolbar({ onRefresh, onOpenFolder, loading, projectPath }: ToolbarProps) {
  return (
    <div className="toolbar" aria-label="Project toolbar">
      <button
        className="toolbar__button"
        onClick={onOpenFolder}
        disabled={loading}
      >
        Open Project
      </button>
      <button
        className="toolbar__button toolbar__button--primary"
        onClick={onRefresh}
        disabled={loading || !projectPath}
      >
        {loading ? 'Scanning...' : 'Refresh'}
      </button>
      {projectPath && (
        <span className="toolbar__path">
          {projectPath}
        </span>
      )}
    </div>
  );
}
