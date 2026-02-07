import type { SessionMeta } from '../../types';

interface SessionListProps {
  sessions: SessionMeta[];
  selected: SessionMeta | null;
  onSelect: (session: SessionMeta) => void;
}

function formatSize(bytes: number): string {
  if (bytes < 1024) return `${bytes}B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)}KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)}MB`;
}

function formatDate(ts: number | null): string {
  if (!ts) return '';
  const d = new Date(ts);
  const now = new Date();
  const diff = now.getTime() - d.getTime();
  const days = Math.floor(diff / (1000 * 60 * 60 * 24));

  if (days === 0) return d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
  if (days === 1) return 'Yesterday';
  if (days < 7) return `${days}d ago`;
  return d.toLocaleDateString([], { month: 'short', day: 'numeric' });
}

export function SessionList({ sessions, selected, onSelect }: SessionListProps) {
  if (sessions.length === 0) {
    return (
      <div className="session-list__empty">
        No sessions found
      </div>
    );
  }

  return (
    <ul className="session-list">
      {sessions.map(session => (
        <li key={session.id}>
          <button
            className={`session-list__item ${selected?.id === session.id ? 'session-list__item--active' : ''}`}
            onClick={() => onSelect(session)}
          >
            <div className="session-list__title">
              {session.title || session.id.slice(0, 8)}
            </div>
            <div className="session-list__meta">
              <span className="session-list__date">{formatDate(session.timestamp)}</span>
              <span className="session-list__badge">{session.message_count} lines</span>
              <span className="session-list__badge">{formatSize(session.file_size)}</span>
            </div>
            {session.git_branch && (
              <div className="session-list__branch">{session.git_branch}</div>
            )}
          </button>
        </li>
      ))}
    </ul>
  );
}
