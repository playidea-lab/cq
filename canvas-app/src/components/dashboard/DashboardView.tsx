import { useEffect } from 'react';
import { useDashboard } from '../../hooks/useDashboard';
import { StatusBadge } from '../shared/StatusBadge';
import { ProgressBar } from '../shared/ProgressBar';
import { TaskList } from './TaskList';
import { TaskDetailPanel } from './TaskDetailPanel';
import { UsagePanel } from './UsagePanel';
import '../../styles/dashboard.css';

interface DashboardViewProps {
  projectPath: string;
}

export function DashboardView({ projectPath }: DashboardViewProps) {
  const {
    state,
    tasks,
    selectedTask,
    loading,
    error,
    loadState,
    loadTaskDetail,
  } = useDashboard();

  useEffect(() => {
    loadState(projectPath);
  }, [projectPath, loadState]);

  if (loading) {
    return <div className="dashboard__loading">Loading project state...</div>;
  }

  if (error) {
    return (
      <div className="dashboard__error">
        <p>Failed to load project state</p>
        <p className="dashboard__error-detail">{error}</p>
      </div>
    );
  }

  if (!state) {
    return <div className="dashboard__empty">No C4 project data found</div>;
  }

  return (
    <div className="dashboard">
      <div className="dashboard__header">
        <div className="dashboard__status">
          <h3 className="dashboard__project-id">{state.project_id}</h3>
          <StatusBadge status={state.status} />
        </div>
        <ProgressBar progress={state.progress} />
        {state.workers.length > 0 && (
          <div className="dashboard__workers">
            {state.workers.map(w => (
              <span key={w.id} className="badge badge--blue">
                {w.id}: {w.current_task || 'idle'}
              </span>
            ))}
          </div>
        )}
        <UsagePanel projectPath={projectPath} />
      </div>

      <div className="dashboard__body">
        <div className="dashboard__task-list-panel">
          <h4 className="dashboard__panel-title">Tasks ({tasks.length})</h4>
          <TaskList
            tasks={tasks}
            selectedId={selectedTask?.id || null}
            onSelect={(id) => loadTaskDetail(projectPath, id)}
          />
        </div>
        <div className="dashboard__task-detail-panel">
          {selectedTask ? (
            <TaskDetailPanel task={selectedTask} />
          ) : (
            <div className="dashboard__placeholder">
              Select a task to view details
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
