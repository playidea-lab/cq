import type { TaskEvent } from '../../types';

interface TaskTimelineProps {
  events: TaskEvent[];
  onSelectTask: (id: string) => void;
}

const STATUS_COLORS: Record<string, string> = {
  done: 'green',
  in_progress: 'blue',
  pending: 'gray',
  blocked: 'red',
};

function formatTimestamp(ts: string | null): string {
  if (!ts) return '';
  try {
    const date = new Date(ts);
    return date.toLocaleString(undefined, {
      month: 'short',
      day: 'numeric',
      hour: '2-digit',
      minute: '2-digit',
    });
  } catch {
    return ts;
  }
}

export function TaskTimeline({ events, onSelectTask }: TaskTimelineProps) {
  if (events.length === 0) {
    return null;
  }

  return (
    <div className="task-timeline">
      <h4 className="task-timeline__title">Recent Activity</h4>
      <div className="task-timeline__list">
        {events.map((event, index) => {
          const color = STATUS_COLORS[event.status] || 'gray';
          return (
            <button
              key={`${event.task_id}-${index}`}
              className="task-timeline__item"
              onClick={() => onSelectTask(event.task_id)}
            >
              <span className={`task-timeline__dot task-timeline__dot--${color}`} />
              <div className="task-timeline__content">
                <div className="task-timeline__row">
                  <span className="badge badge--outline task-timeline__badge">
                    {event.task_id}
                  </span>
                  <span className={`badge badge--${color}`}>
                    {event.status}
                  </span>
                </div>
                <span className="task-timeline__event-title">{event.title}</span>
                {event.updated_at && (
                  <span className="task-timeline__time">
                    {formatTimestamp(event.updated_at)}
                  </span>
                )}
              </div>
            </button>
          );
        })}
      </div>
    </div>
  );
}
