import { useState, useCallback, useEffect } from 'react';
import { open } from '@tauri-apps/plugin-dialog';
import { Sidebar } from './components/Sidebar';
import { SessionsView } from './components/sessions/SessionsView';
import { DashboardView } from './components/dashboard/DashboardView';
import { ConfigView } from './components/config/ConfigView';
import { ErrorBoundary } from './components/shared/ErrorBoundary';
import type { ViewType } from './types';

const VIEW_SHORTCUTS: Record<string, ViewType> = {
  '1': 'sessions',
  '2': 'dashboard',
  '3': 'config',
};

export default function App() {
  const [currentView, setCurrentView] = useState<ViewType>('sessions');
  const [projectPath, setProjectPath] = useState<string | null>(null);

  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && VIEW_SHORTCUTS[e.key]) {
        e.preventDefault();
        setCurrentView(VIEW_SHORTCUTS[e.key]);
      }
    };
    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, []);

  const isTauri = Boolean(
    typeof window !== 'undefined' && (window as any).__TAURI_INTERNALS__
  );

  const handleOpenFolder = useCallback(async () => {
    if (!isTauri) {
      const path = window.prompt('Enter project path:');
      if (path) setProjectPath(path);
      return;
    }

    try {
      const selected = await open({
        directory: true,
        multiple: false,
        title: 'Select C4 Project',
      });
      if (selected && typeof selected === 'string') {
        setProjectPath(selected);
      }
    } catch (err) {
      console.error('Failed to open folder:', err);
    }
  }, [isTauri]);

  const renderView = () => {
    if (!projectPath) {
      return (
        <div className="empty-state">
          <h2 className="empty-state__title">Claude Code Explorer</h2>
          <p className="empty-state__description">
            Browse your Claude Code sessions, C4 workflow status, and project configuration.
          </p>
          <button className="btn btn--primary" onClick={handleOpenFolder}>
            Open Project Folder
          </button>
        </div>
      );
    }

    switch (currentView) {
      case 'sessions':
        return <SessionsView key={`sessions-${projectPath}`} projectPath={projectPath} />;
      case 'dashboard':
        return <DashboardView key={`dashboard-${projectPath}`} projectPath={projectPath} />;
      case 'config':
        return <ConfigView key={`config-${projectPath}`} projectPath={projectPath} />;
    }
  };

  return (
    <div className="app-layout">
      <Sidebar currentView={currentView} onViewChange={setCurrentView} />
      <main className="app-main">
        {projectPath && (
          <header className="app-header">
            <span className="app-header__path" title={projectPath}>
              {projectPath}
            </span>
            <button className="btn btn--secondary btn--sm" onClick={handleOpenFolder}>
              Change
            </button>
          </header>
        )}
        <div className="app-content">
          <ErrorBoundary>
            {renderView()}
          </ErrorBoundary>
        </div>
      </main>
    </div>
  );
}
