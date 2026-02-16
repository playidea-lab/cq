import { useEffect } from 'react';
import { Command } from 'cmdk';
import { useUI } from '../../contexts/UIContext';
import { useAuth } from '../../hooks/useAuth';
import type { ViewType } from '../../types';

interface CommandPaletteProps {
  onViewChange: (view: ViewType) => void;
  onOpenFolder: () => void;
}

export function CommandPalette({ onViewChange, onOpenFolder }: CommandPaletteProps) {
  const { isPaletteOpen, setPaletteOpen, toggleChat } = useUI();
  const { logout } = useAuth();

  useEffect(() => {
    const down = (e: KeyboardEvent) => {
      if (e.key === 'k' && (e.metaKey || e.ctrlKey)) {
        e.preventDefault();
        setPaletteOpen(!isPaletteOpen);
      }
    };

    document.addEventListener('keydown', down);
    return () => document.removeEventListener('keydown', down);
  }, [isPaletteOpen, setPaletteOpen]);

  if (!isPaletteOpen) return null;

  return (
    <div className="command-palette-overlay" onClick={() => setPaletteOpen(false)}>
      <div onClick={(e) => e.stopPropagation()}>
        <Command label="Command Palette">
          <Command.Input placeholder="Type a command or search..." />
          <Command.List>
            <Command.Empty>No results found.</Command.Empty>

            <Command.Group heading="Navigation">
              <Command.Item onSelect={() => { onViewChange('board'); setPaletteOpen(false); }}>
                Dashboard
              </Command.Item>
              <Command.Item onSelect={() => { onViewChange('docs'); setPaletteOpen(false); }}>
                Documents
              </Command.Item>
              <Command.Item onSelect={() => { onViewChange('knowledge'); setPaletteOpen(false); }}>
                Knowledge
              </Command.Item>
              <Command.Item onSelect={() => { onViewChange('messenger'); setPaletteOpen(false); }}>
                Messenger
              </Command.Item>
              <Command.Item onSelect={() => { onViewChange('settings'); setPaletteOpen(false); }}>
                Settings
              </Command.Item>
            </Command.Group>

            <Command.Group heading="Actions">
              <Command.Item onSelect={() => { onOpenFolder(); setPaletteOpen(false); }}>
                Change Project Folder...
              </Command.Item>
              <Command.Item onSelect={() => { toggleChat(); setPaletteOpen(false); }}>
                Toggle Chat Sidebar
              </Command.Item>
            </Command.Group>

            <Command.Group heading="System">
              <Command.Item onSelect={() => { logout(); setPaletteOpen(false); }}>
                Logout
              </Command.Item>
            </Command.Group>
          </Command.List>
        </Command>
      </div>
    </div>
  );
}
