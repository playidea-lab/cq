import { useMemo } from 'react';
import { LayoutDashboard, MessageSquare, Settings, Users } from 'lucide-react';
import type { ViewType } from '../types';
import '../styles/sidebar.css';

interface SidebarProps {
  currentView: ViewType;
  onViewChange: (view: ViewType) => void;
  showTeam?: boolean;
}

const baseNavItems: { view: ViewType; label: string; icon: typeof LayoutDashboard }[] = [
  { view: 'sessions', label: 'Sessions', icon: MessageSquare },
  { view: 'dashboard', label: 'Dashboard', icon: LayoutDashboard },
  { view: 'config', label: 'Config', icon: Settings },
];

const teamNavItem = { view: 'team' as ViewType, label: 'Team', icon: Users };

export function Sidebar({ currentView, onViewChange, showTeam }: SidebarProps) {
  const navItems = useMemo(
    () => (showTeam ? [...baseNavItems, teamNavItem] : baseNavItems),
    [showTeam],
  );

  return (
    <nav className="sidebar" aria-label="Main navigation">
      <div className="sidebar__logo">C4</div>
      <ul className="sidebar__nav">
        {navItems.map(({ view, label, icon: Icon }) => (
          <li key={view}>
            <button
              className={`sidebar__item ${currentView === view ? 'sidebar__item--active' : ''}`}
              onClick={() => onViewChange(view)}
              aria-current={currentView === view ? 'page' : undefined}
              title={label}
            >
              <Icon className="sidebar__icon" size={20} />
              <span className="sidebar__label">{label}</span>
            </button>
          </li>
        ))}
      </ul>
    </nav>
  );
}
