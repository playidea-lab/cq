import { useState, useEffect } from 'react';
import { MessageSquare, LayoutDashboard, FileText, BookOpen, Settings, Sun, Moon, FolderOpen } from 'lucide-react';
import type { WorkspaceMode } from '../types';
import '../styles/workspace-nav.css';

interface WorkspaceNavProps {
  mode: WorkspaceMode;
  onModeChange: (mode: WorkspaceMode) => void;
  projectPath?: string | null;
  onChangeProject?: () => void;
}

const navItems: { mode: WorkspaceMode; label: string; icon: typeof MessageSquare }[] = [
  { mode: 'messenger', label: 'Messenger', icon: MessageSquare },
  { mode: 'board', label: 'Board', icon: LayoutDashboard },
  { mode: 'docs', label: 'Docs', icon: FileText },
  { mode: 'knowledge', label: 'Knowledge', icon: BookOpen },
  { mode: 'settings', label: 'Settings', icon: Settings },
];

function getInitialTheme(): 'dark' | 'light' {
  try {
    return (localStorage.getItem('c1-theme') as 'dark' | 'light') || 'dark';
  } catch {
    return 'dark';
  }
}

export function WorkspaceNav({ mode, onModeChange, projectPath, onChangeProject }: WorkspaceNavProps) {
  const [theme, setTheme] = useState<'dark' | 'light'>(getInitialTheme);

  useEffect(() => {
    document.documentElement.setAttribute('data-theme', theme);
    try {
      localStorage.setItem('c1-theme', theme);
    } catch { /* ignore */ }
  }, [theme]);

  const toggleTheme = () => setTheme(prev => prev === 'dark' ? 'light' : 'dark');

  return (
    <nav className="workspace-nav" aria-label="Workspace navigation">
      <ul className="workspace-nav__list">
        {navItems.map(({ mode: itemMode, label, icon: Icon }) => (
          <li key={itemMode}>
            <button
              className={`workspace-nav__item ${mode === itemMode ? 'workspace-nav__item--active' : ''}`}
              onClick={() => onModeChange(itemMode)}
              aria-current={mode === itemMode ? 'page' : undefined}
              title={label}
            >
              <Icon size={20} />
            </button>
          </li>
        ))}
      </ul>
      <div className="workspace-nav__spacer" />
      {onChangeProject && (
        <button
          className="workspace-nav__theme-toggle"
          onClick={onChangeProject}
          title={projectPath ? `Project: ${projectPath.split('/').pop()}
Click to change` : 'Open project'}
          aria-label="Change project"
        >
          <FolderOpen size={18} />
        </button>
      )}
      <button
        className="workspace-nav__theme-toggle"
        onClick={toggleTheme}
        title={`Switch to ${theme === 'dark' ? 'light' : 'dark'} mode`}
        aria-label={`Switch to ${theme === 'dark' ? 'light' : 'dark'} mode`}
      >
        {theme === 'dark' ? <Sun size={18} /> : <Moon size={18} />}
      </button>
    </nav>
  );
}
