import { useTask } from '../../contexts/TaskContext';
import { useAuth } from '../../hooks/useAuth';
import { useUI } from '../../contexts/UIContext';

interface HeaderProps {
  projectPath: string | null;
  onOpenFolder: () => void;
}

export function Header({ projectPath, onOpenFolder }: HeaderProps) {
  const { user, logout } = useAuth();
  const { activeTask } = useTask();
  const { isChatOpen, toggleChat } = useUI();

  return (
    <header className="app-header">
      <div className="app-header__left">
        {projectPath ? (
          <span className="app-header__path" title={projectPath}>
            {projectPath.split('/').pop() || projectPath}
          </span>
        ) : (
          <span />
        )}
      </div>

      <div className="app-header__center">
        {activeTask ? (
          <div className="task-hud task-hud--active">
            <span className="task-hud__status-dot" />
            <span className="task-hud__id">{activeTask.id}</span>
            <span className="task-hud__title">{activeTask.title}</span>
          </div>
        ) : (
          <div className="task-hud task-hud--idle">
            <span className="task-hud__text">No Active Task</span>
          </div>
        )}
      </div>

      <div className="app-header__right">
        {projectPath && (
          <button 
            className={`btn btn--icon ${isChatOpen ? 'btn--active' : ''}`}
            onClick={toggleChat}
            title="Toggle Messenger"
            style={{ marginRight: '1rem' }}
          >
            <span className="icon">💬</span>
          </button>
        )}
        {user && (
          <div className="app-header__user">
            <span className="app-header__email" title={user.email}>
              {user.email}
            </span>
            <button className="btn btn--secondary btn--sm" onClick={onOpenFolder}>
              Change
            </button>
            <button className="app-header__logout" onClick={logout}>
              Logout
            </button>
          </div>
        )}
      </div>
    </header>
  );
}
