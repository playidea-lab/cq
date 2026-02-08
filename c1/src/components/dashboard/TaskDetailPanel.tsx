import { MarkdownViewer } from '../shared/MarkdownViewer';
import { StatusBadge } from '../shared/StatusBadge';
import type { TaskDetail } from '../../types';

interface TaskDetailPanelProps {
  task: TaskDetail;
}

export function TaskDetailPanel({ task }: TaskDetailPanelProps) {
  return (
    <div className="task-detail">
      <div className="task-detail__header">
        <h3 className="task-detail__title">{task.id}: {task.title}</h3>
        <StatusBadge status={task.status} />
      </div>

      <div className="task-detail__grid">
        <div className="task-detail__field">
          <span className="task-detail__label">Type</span>
          <span className="task-detail__value">{task.task_type}</span>
        </div>
        {task.assigned_to && (
          <div className="task-detail__field">
            <span className="task-detail__label">Assigned</span>
            <span className="task-detail__value">{task.assigned_to}</span>
          </div>
        )}
        {task.domain && (
          <div className="task-detail__field">
            <span className="task-detail__label">Domain</span>
            <span className="task-detail__value">{task.domain}</span>
          </div>
        )}
        {task.scope && (
          <div className="task-detail__field">
            <span className="task-detail__label">Scope</span>
            <span className="task-detail__value task-detail__value--mono">{task.scope}</span>
          </div>
        )}
        {task.branch && (
          <div className="task-detail__field">
            <span className="task-detail__label">Branch</span>
            <span className="task-detail__value task-detail__value--mono">{task.branch}</span>
          </div>
        )}
        {task.commit_sha && (
          <div className="task-detail__field">
            <span className="task-detail__label">Commit</span>
            <span className="task-detail__value task-detail__value--mono">{task.commit_sha}</span>
          </div>
        )}
      </div>

      {task.dependencies.length > 0 && (
        <div className="task-detail__section">
          <h4 className="task-detail__section-title">Dependencies</h4>
          <div className="task-detail__deps">
            {task.dependencies.map(dep => (
              <span key={dep} className="badge badge--outline">{dep}</span>
            ))}
          </div>
        </div>
      )}

      {task.dod && (
        <div className="task-detail__section">
          <h4 className="task-detail__section-title">Definition of Done</h4>
          <MarkdownViewer content={task.dod} className="task-detail__dod" />
        </div>
      )}

      {task.review_decision && (
        <div className="task-detail__section">
          <h4 className="task-detail__section-title">Review Decision</h4>
          <span className={`badge ${task.review_decision === 'APPROVE' ? 'badge--green' : 'badge--red'}`}>
            {task.review_decision}
          </span>
        </div>
      )}
    </div>
  );
}
