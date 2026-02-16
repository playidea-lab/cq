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
import { TaskProvider } from './contexts/TaskContext';
import { UIProvider, useUI } from './contexts/UIContext';
import { Header } from './components/layout/Header';
import { MainLayout } from './components/layout/MainLayout';
import { ChatDrawer } from './components/channels/ChatDrawer';
import { CommandPalette } from './components/shared/CommandPalette';
import { useAuth } from './hooks/useAuth';
import type { ViewType } from './types';
import './styles/auth.css';
import './styles/layout.css';

// ... (skipping unchanged code for brevity in replace tool)

  return (
    <>
      <MainLayout
        sidebar={<Sidebar currentView={currentView} onViewChange={setCurrentView} />}
        header={<Header projectPath={projectPath} onOpenFolder={handleOpenFolder} />}
        content={<ErrorBoundary>{renderView()}</ErrorBoundary>}
        drawer={projectPath ? <ChatDrawer projectPath={projectPath} /> : undefined}
        isDrawerOpen={isChatOpen}
      />
      <CommandPalette 
        onViewChange={setCurrentView} 
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


