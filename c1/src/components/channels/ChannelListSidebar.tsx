import { useState, useCallback, useMemo } from 'react';
import type { Channel } from '../../types';

type SectionConfig = {
  key: string;
  label: string;
  icon: string;
  match: (ch: Channel) => boolean;
};

const GENERAL_NAMES = ['general', 'tasks', 'events', 'knowledge'];

const SECTIONS: SectionConfig[] = [
  {
    key: 'general',
    label: 'General',
    icon: '#',
    match: (ch) => GENERAL_NAMES.includes(ch.name) || ch.channel_type === 'auto',
  },
  {
    key: 'project',
    label: 'Projects',
    icon: '📂',
    match: (ch) =>
      ch.channel_type === 'topic' &&
      !GENERAL_NAMES.includes(ch.name) &&
      ch.name.startsWith('project-'),
  },
  {
    key: 'knowledge',
    label: 'Knowledge',
    icon: '🧠',
    match: (ch) =>
      ch.channel_type === 'topic' &&
      !GENERAL_NAMES.includes(ch.name) &&
      ch.name.startsWith('knowledge-'),
  },
  {
    key: 'session',
    label: 'Sessions',
    icon: '💬',
    match: (ch) => ch.channel_type === 'worker',
  },
  {
    key: 'dm',
    label: 'Direct',
    icon: '✉',
    match: (ch) => ch.channel_type === 'dm',
  },
];

interface ChannelListSidebarProps {
  channels: Channel[];
  selectedChannel: Channel | null;
  onSelect: (ch: Channel) => void;
  onCreate: (name: string, type: string) => void;
}

export function ChannelListSidebar({
  channels,
  selectedChannel,
  onSelect,
  onCreate,
}: ChannelListSidebarProps) {
  const [collapsed, setCollapsed] = useState<Record<string, boolean>>({});
  const [showModal, setShowModal] = useState(false);
  const [newName, setNewName] = useState('');
  const [newType, setNewType] = useState('topic');

  const toggleSection = useCallback((key: string) => {
    setCollapsed((prev) => ({ ...prev, [key]: !prev[key] }));
  }, []);

  const handleChannelKeyDown = useCallback(
    (ch: Channel, e: React.KeyboardEvent) => {
      if (e.key === 'Enter' || e.key === ' ') {
        e.preventDefault();
        onSelect(ch);
      }
    },
    [onSelect],
  );

  const handleCreate = useCallback(() => {
    if (!newName.trim()) return;
    onCreate(newName.trim(), newType);
    setShowModal(false);
    setNewName('');
    setNewType('topic');
  }, [newName, newType, onCreate]);

  // Assign channels to sections. A channel can only appear in the first matching section.
  const sectionChannels = useMemo(() => {
    const assigned = new Set<string>();
    const result: Record<string, Channel[]> = {};

    for (const section of SECTIONS) {
      result[section.key] = [];
      for (const ch of channels) {
        if (!assigned.has(ch.id) && section.match(ch)) {
          result[section.key].push(ch);
          assigned.add(ch.id);
        }
      }
    }

    // Channels that didn't match any section go into 'general'
    for (const ch of channels) {
      if (!assigned.has(ch.id)) {
        result['general'].push(ch);
      }
    }

    return result;
  }, [channels]);

  return (
    <>
      <aside className="channel-list-sidebar">
        <div className="channel-list-sidebar__header">
          <span className="channel-list-sidebar__title">Channels</span>
          <button
            className="channel-list-sidebar__add-btn"
            onClick={() => setShowModal(true)}
            title="Create channel"
          >
            +
          </button>
        </div>

        <div className="channel-list-sidebar__sections">
          {SECTIONS.map((section) => {
            const sectionChs = sectionChannels[section.key] ?? [];
            const isCollapsed = !!collapsed[section.key];

            return (
              <div key={section.key} className="channel-list-sidebar__section">
                <button
                  className="channel-list-sidebar__section-header"
                  onClick={() => toggleSection(section.key)}
                  aria-expanded={!isCollapsed}
                >
                  <span className="channel-list-sidebar__section-icon">
                    {section.icon}
                  </span>
                  <span className="channel-list-sidebar__section-title">
                    {section.label}
                  </span>
                  <span className="channel-list-sidebar__section-toggle">
                    {isCollapsed ? '▸' : '▾'}
                  </span>
                </button>

                {!isCollapsed && (
                  <ul className="channel-list-sidebar__list">
                    {sectionChs.length === 0 ? (
                      <li className="channel-list-sidebar__empty">
                        No {section.label.toLowerCase()} channels
                      </li>
                    ) : (
                      sectionChs.map((ch) => (
                        <li
                          key={ch.id}
                          className={`channel-list-sidebar__channel${
                            selectedChannel?.id === ch.id
                              ? ' channel-list-sidebar__channel--active'
                              : ''
                          }`}
                          onClick={() => onSelect(ch)}
                          role="button"
                          tabIndex={0}
                          onKeyDown={(e) => handleChannelKeyDown(ch, e)}
                        >
                          <span className="channel-list-sidebar__channel-icon">
                            #
                          </span>
                          <span className="channel-list-sidebar__channel-name">
                            {ch.name}
                          </span>
                        </li>
                      ))
                    )}
                  </ul>
                )}
              </div>
            );
          })}
        </div>
      </aside>

      {showModal && (
        <div className="create-channel-modal" onClick={() => setShowModal(false)}>
          <div
            className="create-channel-modal__content"
            onClick={(e) => e.stopPropagation()}
          >
            <h3 className="create-channel-modal__title">Create Channel</h3>
            <div className="create-channel-modal__field">
              <label className="create-channel-modal__label">Name</label>
              <input
                className="create-channel-modal__input"
                value={newName}
                onChange={(e) => setNewName(e.target.value)}
                placeholder="e.g. project-alpha, knowledge-api"
                autoFocus
                onKeyDown={(e) => {
                  if (e.key === 'Enter') handleCreate();
                }}
              />
            </div>
            <div className="create-channel-modal__field">
              <label className="create-channel-modal__label">Type</label>
              <select
                className="create-channel-modal__input"
                value={newType}
                onChange={(e) => setNewType(e.target.value)}
              >
                <option value="topic">Topic</option>
                <option value="dm">Direct Message</option>
              </select>
            </div>
            <div className="create-channel-modal__actions">
              <button
                className="btn btn--secondary"
                onClick={() => setShowModal(false)}
              >
                Cancel
              </button>
              <button
                className="btn btn--primary"
                onClick={handleCreate}
                disabled={!newName.trim()}
              >
                Create
              </button>
            </div>
          </div>
        </div>
      )}
    </>
  );
}
