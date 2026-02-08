import { useRef } from 'react';
import { useVirtualizer } from '@tanstack/react-virtual';
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
  const parentRef = useRef<HTMLDivElement>(null);

  const virtualizer = useVirtualizer({
    count: tasks.length,
    getScrollElement: () => parentRef.current,
    estimateSize: () => 72,
    overscan: 5,
  });

  if (tasks.length === 0) {
    return <div className="task-list__empty">No tasks found</div>;
  }

  return (
    <div ref={parentRef} className="task-list">
      <div
        style={{
          height: `${virtualizer.getTotalSize()}px`,
          width: '100%',
          position: 'relative',
        }}
      >
        {virtualizer.getVirtualItems().map(virtualItem => {
          const task = tasks[virtualItem.index];
          return (
            <div
              key={task.id}
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
            </div>
          );
        })}
      </div>
    </div>
  );
}
