import { useEffect, useState, useCallback } from 'react';
import { invoke } from '@tauri-apps/api/core';
import { useDashboard } from '../../hooks/useDashboard';
import { StatusBadge } from '../shared/StatusBadge';
import { ProgressBar } from '../shared/ProgressBar';
import { TaskList } from './TaskList';
import { TaskDetailPanel } from './TaskDetailPanel';
import { TaskTimeline } from './TaskTimeline';
import { ValidationPanel } from './ValidationPanel';
import { UsagePanel } from './UsagePanel';
import type { SyncResult } from '../../types';
import '../../styles/dashboard.css';

interface DashboardViewProps {
  projectPath: string;
}

export function DashboardView({ projectPath }: DashboardViewProps) {
  const {
    state,
    tasks,
    selectedTask,
    timeline,
    validations,
    loading,
    error,
    loadState,
    loadTimeline,
    loadValidations,
    loadTaskDetail,
  } = useDashboard();

  const [syncing, setSyncing] = useState(false);
  const [lastSynced, setLastSynced] = useState<string | null>(null);
  const [syncError, setSyncError] = useState<string | null>(null);

  useEffect(() => {
    loadState(projectPath);
    loadTimeline(projectPath);
  }, [projectPath, loadState, loadTimeline]);

  // Load validations when a task is selected and has validations
  useEffect(() => {
    if (selectedTask && selectedTask.validations.length > 0) {
      loadValidations(projectPath, selectedTask.id);
    }
  }, [selectedTask, projectPath, loadValidations]);

  const handleSelectTask = (taskId: string) => {
    loadTaskDetail(projectPath, taskId);
  };

  const handleSync = useCallback(async () => {
    setSyncing(true);
    setSyncError(null);
    try {
      const result = await invoke<SyncResult>('cloud_sync_tasks', {
        projectPath,
      });
      setLastSynced(result.last_synced);
      if (result.errors.length > 0) {
        setSyncError(result.errors.join('; '));
      }
    } catch (err) {
      setSyncError(err instanceof Error ? err.message : String(err));
    } finally {
      setSyncing(false);
    }
  }, [projectPath]);

  const formatSyncTime = (iso: string): string => {
    try {
      const d = new Date(iso);
      return d.toLocaleTimeString(undefined, {
        hour: '2-digit',
        minute: '2-digit',
      });
    } catch {
      return iso;
    }
  };

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
          <div className="dashboard__sync">
            <button
              className={`btn btn--secondary btn--sm sync-btn ${syncing ? 'sync-btn--syncing' : ''}`}
              onClick={handleSync}
              disabled={syncing}
            >
              {syncing ? 'Syncing...' : 'Sync to Cloud'}
            </button>
            {lastSynced && (
              <span className="sync-btn__time">
                Last sync: {formatSyncTime(lastSynced)}
              </span>
            )}
            {syncError && (
              <span className="sync-btn__error" title={syncError}>
                Sync error
              </span>
            )}
          </div>
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

      {timeline.length > 0 && (
        <TaskTimeline events={timeline} onSelectTask={handleSelectTask} />
      )}

      <div className="dashboard__body">
        <div className="dashboard__task-list-panel">
          <h4 className="dashboard__panel-title">Tasks ({tasks.length})</h4>
          <TaskList
            tasks={tasks}
            selectedId={selectedTask?.id || null}
            onSelect={handleSelectTask}
          />
        </div>
        <div className="dashboard__task-detail-panel">
          {selectedTask ? (
            <>
              <TaskDetailPanel task={selectedTask} />
              {validations.length > 0 && (
                <ValidationPanel validations={validations} />
              )}
            </>
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
