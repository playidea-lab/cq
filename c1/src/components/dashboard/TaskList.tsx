import { StatusBadge } from '../shared/StatusBadge';
import type { TaskItem } from '../../types';

interface TaskListProps {
  tasks: TaskItem[];
  selectedId: string | null;
  onSelect: (taskId: string) => void;
}

const TYPE_LABELS: Record<string, string> = {
  IMPLEMENTATION: 'impl',
  REVIEW: 'review',
  CHECKPOINT: 'cp',
};

export function TaskList({ tasks, selectedId, onSelect }: TaskListProps) {
  if (tasks.length === 0) {
    return <div className="task-list__empty">No tasks found</div>;
  }

  return (
    <ul className="task-list">
      {tasks.map(task => (
        <li key={task.id}>
          <button
            className={`task-list__item ${selectedId === task.id ? 'task-list__item--active' : ''}`}
            onClick={() => onSelect(task.id)}
          >
            <div className="task-list__header">
              <span className="task-list__id">{task.id}</span>
              <StatusBadge status={task.status} />
            </div>
            <div className="task-list__title">{task.title}</div>
            <div className="task-list__meta">
              <span className="badge badge--outline">
                {TYPE_LABELS[task.task_type] || task.task_type}
              </span>
              {task.assigned_to && (
                <span className="task-list__assigned">{task.assigned_to}</span>
              )}
              {task.domain && (
                <span className="task-list__domain">{task.domain}</span>
              )}
            </div>
          </button>
        </li>
      ))}
    </ul>
  );
}
