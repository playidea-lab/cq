import { useState, useEffect, useCallback } from 'react';
import { invoke } from '@tauri-apps/api/core';
import { StatusBadge } from '../shared/StatusBadge';
import { ProgressBar } from '../shared/ProgressBar';
import type { TeamProject, ProjectState } from '../../types';
import '../../styles/team.css';

export function TeamView() {
  const [projects, setProjects] = useState<TeamProject[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [selectedProject, setSelectedProject] = useState<string | null>(null);
  const [remoteDashboard, setRemoteDashboard] = useState<ProjectState | null>(null);
  const [dashboardLoading, setDashboardLoading] = useState(false);

  const loadProjects = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const result = await invoke<TeamProject[]>('cloud_get_team_projects');
      setProjects(result);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    loadProjects();
  }, [loadProjects]);

  const handleSelectProject = useCallback(async (projectId: string) => {
    if (selectedProject === projectId) {
      setSelectedProject(null);
      setRemoteDashboard(null);
      return;
    }
    setSelectedProject(projectId);
    setDashboardLoading(true);
    setRemoteDashboard(null);
    try {
      const state = await invoke<ProjectState>('cloud_get_remote_dashboard', {
        projectId,
      });
      setRemoteDashboard(state);
    } catch (err) {
      console.error('Failed to load remote dashboard:', err);
    } finally {
      setDashboardLoading(false);
    }
  }, [selectedProject]);

  const formatDate = (dateStr: string | null): string => {
    if (!dateStr) return 'Never';
    try {
      const d = new Date(dateStr);
      return d.toLocaleDateString(undefined, {
        month: 'short',
        day: 'numeric',
        hour: '2-digit',
        minute: '2-digit',
      });
    } catch {
      return dateStr;
    }
  };

  if (loading) {
    return <div className="team-view__loading">Loading team projects...</div>;
  }

  if (error) {
    return (
      <div className="team-view__error">
        <p className="team-view__error-title">Failed to load team projects</p>
        <p className="team-view__error-detail">{error}</p>
        <button className="btn btn--secondary btn--sm" onClick={loadProjects}>
          Retry
        </button>
      </div>
    );
  }

  if (projects.length === 0) {
    return (
      <div className="team-view__empty">
        <h3 className="team-view__empty-title">No Team Projects</h3>
        <p className="team-view__empty-description">
          Team projects will appear here once project data is synced to the cloud.
          Use the Sync button in the Dashboard view to upload your project.
        </p>
        <button className="btn btn--secondary btn--sm" onClick={loadProjects}>
          Refresh
        </button>
      </div>
    );
  }

  return (
    <div className="team-view">
      <div className="team-view__header">
        <h3 className="team-view__title">Team Projects</h3>
        <span className="team-view__count">{projects.length} projects</span>
        <button className="btn btn--secondary btn--sm" onClick={loadProjects}>
          Refresh
        </button>
      </div>

      <div className="team-view__grid">
        {projects.map((project) => {
          const progress = project.task_count > 0
            ? Math.round((project.done_count / project.task_count) * 100)
            : 0;
          const isSelected = selectedProject === project.id;

          return (
            <div key={project.id} className="team-project-wrapper">
              <button
                className={`team-project ${isSelected ? 'team-project--selected' : ''}`}
                onClick={() => handleSelectProject(project.id)}
                aria-expanded={isSelected}
              >
                <div className="team-project__header">
                  <span className="team-project__name">{project.name}</span>
                  <StatusBadge status={project.status} />
                </div>

                <div className="team-project__owner">{project.owner_email}</div>

                <div className="team-project__progress">
                  <div className="team-project__progress-track">
                    <div
                      className="team-project__progress-fill"
                      style={{ width: `${progress}%` }}
                    />
                  </div>
                  <span className="team-project__progress-label">{progress}%</span>
                </div>

                <div className="team-project__stats">
                  <span className="team-project__stat">
                    {project.done_count}/{project.task_count} tasks
                  </span>
                  <span className="team-project__date">
                    {formatDate(project.last_updated)}
                  </span>
                </div>
              </button>

              {isSelected && (
                <div className="team-project__dashboard">
                  {dashboardLoading ? (
                    <div className="team-project__dashboard-loading">
                      Loading dashboard...
                    </div>
                  ) : remoteDashboard ? (
                    <div className="team-project__dashboard-content">
                      <div className="team-project__dashboard-status">
                        <span className="team-project__dashboard-label">Status</span>
                        <StatusBadge status={remoteDashboard.status} />
                      </div>
                      <ProgressBar progress={remoteDashboard.progress} />
                      <div className="team-project__dashboard-stats">
                        <div className="team-project__dashboard-stat">
                          <span className="team-project__dashboard-stat-value">
                            {remoteDashboard.progress.done}
                          </span>
                          <span className="team-project__dashboard-stat-label">Done</span>
                        </div>
                        <div className="team-project__dashboard-stat">
                          <span className="team-project__dashboard-stat-value">
                            {remoteDashboard.progress.in_progress}
                          </span>
                          <span className="team-project__dashboard-stat-label">Active</span>
                        </div>
                        <div className="team-project__dashboard-stat">
                          <span className="team-project__dashboard-stat-value">
                            {remoteDashboard.progress.pending}
                          </span>
                          <span className="team-project__dashboard-stat-label">Pending</span>
                        </div>
                        <div className="team-project__dashboard-stat">
                          <span className="team-project__dashboard-stat-value">
                            {remoteDashboard.progress.blocked}
                          </span>
                          <span className="team-project__dashboard-stat-label">Blocked</span>
                        </div>
                      </div>
                    </div>
                  ) : (
                    <div className="team-project__dashboard-empty">
                      No dashboard data available
                    </div>
                  )}
                </div>
              )}
            </div>
          );
        })}
      </div>
    </div>
  );
}
