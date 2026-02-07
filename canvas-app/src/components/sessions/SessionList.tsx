import type { SessionMeta } from '../../types';
import { formatSize, formatDate } from '../../utils/format';

interface SessionListProps {
  sessions: SessionMeta[];
  selected: SessionMeta | null;
  onSelect: (session: SessionMeta) => void;
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
