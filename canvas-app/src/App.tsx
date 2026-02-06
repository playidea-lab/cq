import { useState, useCallback } from 'react';
import { open } from '@tauri-apps/plugin-dialog';
import { Sidebar } from './components/Sidebar';
import { DashboardView } from './components/dashboard/DashboardView';
import { RegistryView } from './components/registry/RegistryView';
import { TimelineView } from './components/timeline/TimelineView';
import type { ViewType } from './types';

export default function App() {
  const [currentView, setCurrentView] = useState<ViewType>('dashboard');
  const [projectPath, setProjectPath] = useState<string | null>(null);

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
          <h2 className="empty-state__title">C4 Dashboard</h2>
          <p className="empty-state__description">
            Select a project folder to view your C4 workflow status, tasks, and dependencies.
          </p>
          <button className="btn btn--primary" onClick={handleOpenFolder}>
            Open Project Folder
          </button>
        </div>
      );
    }

    switch (currentView) {
      case 'dashboard':
        return <DashboardView projectPath={projectPath} />;
      case 'registry':
        return <RegistryView projectPath={projectPath} />;
      case 'timeline':
        return <TimelineView projectPath={projectPath} />;
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
          {renderView()}
        </div>
      </main>
    </div>
  );
}
