import { useRef } from 'react';
import { useVirtualizer } from '@tanstack/react-virtual';
import type { SessionMeta } from '../../types';
import { formatSize, formatDate } from '../../utils/format';

interface SessionListProps {
  sessions: SessionMeta[];
  selected: SessionMeta | null;
  onSelect: (session: SessionMeta) => void;
}

export function SessionList({ sessions, selected, onSelect }: SessionListProps) {
  const parentRef = useRef<HTMLDivElement>(null);

  const virtualizer = useVirtualizer({
    count: sessions.length,
    getScrollElement: () => parentRef.current,
    estimateSize: () => 68,
    overscan: 5,
  });

  if (sessions.length === 0) {
    return (
      <div className="session-list__empty">
        No sessions found
      </div>
    );
  }

  return (
    <div ref={parentRef} className="session-list">
      <div
        style={{
          height: `${virtualizer.getTotalSize()}px`,
          width: '100%',
          position: 'relative',
        }}
      >
        {virtualizer.getVirtualItems().map(virtualItem => {
          const session = sessions[virtualItem.index];
          return (
            <div
              key={session.id}
              style={{
                position: 'absolute',
                top: 0,
                left: 0,
                width: '100%',
                height: `${virtualItem.size}px`,
                transform: `translateY(${virtualItem.start}px)`,
              }}
            >
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
            </div>
          );
        })}
      </div>
    </div>
  );
}
