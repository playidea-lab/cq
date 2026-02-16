import { useEffect, useState, useCallback, useRef } from 'react';
import { invoke } from '@tauri-apps/api/core';
import { useDashboard } from '../../hooks/useDashboard';
import { useRealtimeSync, type ConnectionStatus } from '../../hooks/useRealtimeSync';
import { StatusBadge } from '../shared/StatusBadge';
import { ProgressBar } from '../shared/ProgressBar';
import { Skeleton } from '../shared/Skeleton';
import { ErrorState } from '../shared/ErrorState';
import { TaskList } from './TaskList';
import { TaskDetailPanel } from './TaskDetailPanel';
import { GitGraph } from './GitGraph';
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
    gitGraph,
    hasMoreCommits,
    validations,
    loading,
    error,
    loadState,
    loadGitGraph,
    loadMoreGitGraph,
    loadValidations,
    clearValidations,
    loadTaskDetail,
  } = useDashboard();

  const [syncing, setSyncing] = useState(false);
  const [lastSynced, setLastSynced] = useState<string | null>(null);
  const [syncError, setSyncError] = useState<string | null>(null);
  const [autoSync, setAutoSync] = useState(false);
  const autoSyncRef = useRef<ReturnType<typeof setInterval> | null>(null);

  // Realtime subscription — auto-refresh on cloud changes
  const { status: realtimeStatus } = useRealtimeSync({
    autoConnect: true,
    onUpdate: (event) => {
      if (event.table === 'c4_tasks' || event.table === 'c4_state') {
        loadState(projectPath);
      }
    },
  });

  useEffect(() => {
    loadState(projectPath);
    loadGitGraph(projectPath);
  }, [projectPath, loadState, loadGitGraph]);

  // Auto-sync interval (every 30 seconds)
  useEffect(() => {
    if (autoSync) {
      autoSyncRef.current = setInterval(() => {
        handleSync();
      }, 30_000);
    } else if (autoSyncRef.current) {
      clearInterval(autoSyncRef.current);
      autoSyncRef.current = null;
    }
    return () => {
      if (autoSyncRef.current) clearInterval(autoSyncRef.current);
    };
  }, [autoSync]); // eslint-disable-line react-hooks/exhaustive-deps

  useEffect(() => {
    if (selectedTask && selectedTask.validations.length > 0) {
      loadValidations(projectPath, selectedTask.id);
    } else {
      clearValidations();
    }
  }, [selectedTask, projectPath, loadValidations, clearValidations]);

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

  const statusIndicator = (s: ConnectionStatus) => {
    switch (s) {
      case 'connected': return 'sync-status--online';
      case 'connecting':
      case 'reconnecting': return 'sync-status--syncing';
      default: return 'sync-status--offline';
    }
  };

  const statusLabel = (s: ConnectionStatus) => {
    switch (s) {
      case 'connected': return 'Online';
      case 'connecting': return 'Connecting...';
      case 'reconnecting': return 'Reconnecting...';
      default: return 'Offline';
    }
  };

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
    return (
      <div className="dashboard">
        <div className="dashboard__header">
          <Skeleton variant="card" count={1} />
        </div>
        <div className="dashboard__body">
          <div className="dashboard__task-list-panel">
            <Skeleton variant="list-item" count={5} />
          </div>
          <div className="dashboard__task-detail-panel">
            <Skeleton variant="card" count={2} />
          </div>
        </div>
      </div>
    );
  }

  if (error) {
    return (
      <ErrorState
        message="Failed to load project state"
        detail={error}
        onRetry={() => { loadState(projectPath); loadGitGraph(projectPath); }}
      />
    );
  }

  if (!state) {
    return (
      <div className="dashboard__empty">
        <p>No C4 project data found</p>
        <p className="dashboard__empty-hint">Open a project with a .c4/ directory to see dashboard data.</p>
      </div>
    );
  }

  return (
    <div className="dashboard">
      <div className="dashboard__header">
        <div className="dashboard__status">
          <h3 className="dashboard__project-id">{state.project_id}</h3>
          <StatusBadge status={state.status} />
          <div className="dashboard__sync">
            <span className={`sync-status ${statusIndicator(realtimeStatus)}`}
              title={statusLabel(realtimeStatus)}>
              {statusLabel(realtimeStatus)}
            </span>
            <button
              className={`btn btn--secondary btn--sm sync-btn ${syncing ? 'sync-btn--syncing' : ''}`}
              onClick={handleSync}
              disabled={syncing}
            >
              {syncing ? 'Syncing...' : 'Sync to Cloud'}
            </button>
            <label className="sync-auto-toggle" title="Auto-sync every 30 seconds">
              <input
                type="checkbox"
                checked={autoSync}
                onChange={(e) => setAutoSync(e.target.checked)}
              />
              Auto
            </label>
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

      {gitGraph.length > 0 && (
        <GitGraph
          commits={gitGraph}
          hasMore={hasMoreCommits}
          onLoadMore={() => loadMoreGitGraph(projectPath)}
        />
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
