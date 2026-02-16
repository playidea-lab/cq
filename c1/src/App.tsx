import { useState, useCallback, useEffect } from 'react';
import { open } from '@tauri-apps/plugin-dialog';
import { Sidebar } from './components/Sidebar';
import { DashboardView } from './components/dashboard/DashboardView';
import { DocumentsView } from './components/documents/DocumentsView';
import { KnowledgeView } from './components/knowledge/KnowledgeView';
import { ChannelsView } from './components/channels/ChannelsView';
import { ConfigView } from './components/config/ConfigView';
import { LoginView } from './components/auth/LoginView';
import { ErrorBoundary } from './components/shared/ErrorBoundary';
import { AuthProvider } from './contexts/AuthContext';
import { ToastProvider } from './contexts/ToastContext';
import { useAuth } from './hooks/useAuth';
import type { ViewType } from './types';
import './styles/auth.css';

const VIEW_SHORTCUTS: Record<string, ViewType> = {
  '1': 'board',
  '2': 'docs',
  '3': 'knowledge',
  '4': 'messenger',
  '5': 'settings',
};

function AppContent() {
  const { user, loading, logout } = useAuth();
  const [currentView, setCurrentView] = useState<ViewType>('board');
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

  if (loading) {
    return (
      <div className="app-layout">
        <Sidebar currentView={currentView} onViewChange={setCurrentView} />
        <main className="app-main">
          <div className="app-content">
            <div className="empty-state">
              <p className="empty-state__description">Loading...</p>
            </div>
          </div>
        </main>
      </div>
    );
  }

  if (!user) {
    return (
      <div className="app-layout">
        <Sidebar currentView={currentView} onViewChange={setCurrentView} />
        <main className="app-main">
          <div className="app-content">
            <LoginView />
          </div>
        </main>
      </div>
    );
  }

  const renderView = () => {
    if (!projectPath) {
      return (
        <div className="empty-state">
          <h2 className="empty-state__title">C4 Board</h2>
          <p className="empty-state__description">
            Manage your C4 project: tasks, documents, knowledge, and team communication.
          </p>
          <button className="btn btn--primary" onClick={handleOpenFolder}>
            Open Project Folder
          </button>
        </div>
      );
    }

    switch (currentView) {
      case 'board':
        return <DashboardView key={`board-${projectPath}`} projectPath={projectPath} />;
      case 'docs':
        return <DocumentsView key={`docs-${projectPath}`} projectPath={projectPath} />;
      case 'knowledge':
        return <KnowledgeView key={`knowledge-${projectPath}`} projectPath={projectPath} />;
      case 'messenger':
        return <ChannelsView key={`messenger-${projectPath}`} projectId={projectPath} />;
      case 'settings':
        return <ConfigView key={`settings-${projectPath}`} projectPath={projectPath} />;
    }
  };

  return (
    <div className="app-layout">
      <Sidebar
        currentView={currentView}
        onViewChange={setCurrentView}
      />
      <main className="app-main">
        <header className="app-header">
          {projectPath ? (
            <>
              <span className="app-header__path" title={projectPath}>
                {projectPath}
              </span>
              <div className="app-header__user">
                <span className="app-header__email" title={user.email}>
                  {user.email}
                </span>
                <button className="btn btn--secondary btn--sm" onClick={handleOpenFolder}>
                  Change
                </button>
                <button className="app-header__logout" onClick={logout}>
                  Logout
                </button>
              </div>
            </>
          ) : (
            <>
              <span />
              <div className="app-header__user">
                <span className="app-header__email" title={user.email}>
                  {user.email}
                </span>
                <button className="app-header__logout" onClick={logout}>
                  Logout
                </button>
              </div>
            </>
          )}
        </header>
        <div className="app-content">
          <ErrorBoundary>
            {renderView()}
          </ErrorBoundary>
        </div>
      </main>
    </div>
  );
}

export default function App() {
  return (
    <AuthProvider>
      <ToastProvider>
        <AppContent />
      </ToastProvider>
    </AuthProvider>
  );
}
