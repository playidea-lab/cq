import { useState, useCallback, useEffect } from 'react';
import { open } from '@tauri-apps/plugin-dialog';
import { WorkspaceNav } from './components/WorkspaceNav';
import { DashboardView } from './components/dashboard/DashboardView';
import { DocumentsView } from './components/documents/DocumentsView';
import { KnowledgeView } from './components/knowledge/KnowledgeView';
import { ConfigView } from './components/config/ConfigView';
import { LoginView } from './components/auth/LoginView';
import { ErrorBoundary } from './components/shared/ErrorBoundary';
import { AuthProvider } from './contexts/AuthContext';
import { ToastProvider } from './contexts/ToastContext';
import { TaskProvider, useTask } from './contexts/TaskContext';
import { UIProvider } from './contexts/UIContext';
import { MainLayout } from './components/layout/MainLayout';
import { CommandPalette } from './components/shared/CommandPalette';
import { useAuth } from './hooks/useAuth';
import type { WorkspaceMode } from './types';
import './styles/auth.css';
import './styles/layout.css';

const MODE_SHORTCUTS: Record<string, WorkspaceMode> = {
  '1': 'messenger',
  '2': 'board',
  '3': 'docs',
  '4': 'knowledge',
  '5': 'settings',
};

// Component to handle task polling based on projectPath
function TaskPoller({ projectPath }: { projectPath: string | null }) {
  const { refresh } = useTask();

  useEffect(() => {
    refresh(projectPath);
    if (!projectPath) return;

    const interval = setInterval(() => {
      refresh(projectPath);
    }, 5000); // Poll every 5s

    return () => clearInterval(interval);
  }, [projectPath, refresh]);

  return null;
}

function AppContent() {
  const { user, loading } = useAuth();
  const [workspaceMode, setWorkspaceMode] = useState<WorkspaceMode>('messenger');
  const [projectPath, setProjectPath] = useState<string | null>(null);

  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && MODE_SHORTCUTS[e.key]) {
        e.preventDefault();
        setWorkspaceMode(MODE_SHORTCUTS[e.key]);
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

  // Render the channel list area based on workspace mode.
  // When mode is 'messenger', show channel list placeholder (ChannelListSidebar in T-943).
  // Other modes render their secondary view here.
  const renderChannelList = () => {
    if (workspaceMode === 'messenger') {
      // Placeholder: ChannelListSidebar will be wired in T-943
      return (
        <div className="channel-list-placeholder">
          {/* Channel list will be rendered in T-943 */}
        </div>
      );
    }
    // Non-messenger modes: show the relevant view in the channel list area
    if (!projectPath) {
      return (
        <div className="channel-list-placeholder" />
      );
    }
    switch (workspaceMode) {
      case 'board':
        return <DashboardView key={`board-${projectPath}`} projectPath={projectPath} />;
      case 'docs':
        return <DocumentsView key={`docs-${projectPath}`} projectPath={projectPath} />;
      case 'knowledge':
        return <KnowledgeView key={`knowledge-${projectPath}`} projectPath={projectPath} />;
      case 'settings':
        return <ConfigView key={`settings-${projectPath}`} projectPath={projectPath} />;
    }
  };

  // Content area: always show main content
  const renderContent = () => {
    if (loading) {
      return <div className="empty-state">Loading...</div>;
    }

    if (!user) {
      return <LoginView />;
    }

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

    // NOTE: ChannelsView is not rendered here yet (done in T-945)
    return null;
  };

  const workspaceNav = (
    <WorkspaceNav mode={workspaceMode} onModeChange={setWorkspaceMode} />
  );

  return (
    <>
      <TaskPoller projectPath={projectPath} />
      <MainLayout
        leftNav={workspaceNav}
        channelList={renderChannelList()}
        content={<ErrorBoundary>{renderContent()}</ErrorBoundary>}
      />
      <CommandPalette
        onViewChange={(view) => setWorkspaceMode(view as WorkspaceMode)}
        onOpenFolder={handleOpenFolder}
      />
    </>
  );
}

export default function App() {
  return (
    <AuthProvider>
      <ToastProvider>
        <UIProvider>
          <TaskProvider>
            <AppContent />
          </TaskProvider>
        </UIProvider>
      </ToastProvider>
    </AuthProvider>
  );
}
