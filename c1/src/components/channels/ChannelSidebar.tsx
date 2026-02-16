import { useState, useCallback, useMemo } from 'react';
import type { Channel, C1Member } from '../../types';

const SYSTEM_CHANNELS = ['general', 'tasks', 'events', 'knowledge'];

interface ChannelSidebarProps {
  channels: Channel[];
  selectedChannel: Channel | null;
  loading: boolean;
  onSelect: (channel: Channel) => void;
  onCreate: (name: string, description: string, channelType: string) => Promise<Channel | null>;
  members?: C1Member[];
}

export function ChannelSidebar({
  channels,
  selectedChannel,
  loading,
  onSelect,
  onCreate,
  members = [],
}: ChannelSidebarProps) {
  const [showModal, setShowModal] = useState(false);
  const [newName, setNewName] = useState('');
  const [newDesc, setNewDesc] = useState('');
  const [creating, setCreating] = useState(false);

  const handleCreate = useCallback(async () => {
    if (!newName.trim()) return;
    setCreating(true);
    const channel = await onCreate(newName.trim(), newDesc.trim(), 'topic');
    setCreating(false);
    if (channel) {
      setShowModal(false);
      setNewName('');
      setNewDesc('');
      onSelect(channel);
    }
  }, [newName, newDesc, onCreate, onSelect]);

  const handleChannelKeyDown = useCallback((channel: Channel, e: React.KeyboardEvent) => {
    if (e.key === 'Enter' || e.key === ' ') {
      e.preventDefault();
      onSelect(channel);
    }
  }, [onSelect]);

  // Group channels into system and user-created
  const { systemChannels, userChannels } = useMemo(() => {
    const sys: Channel[] = [];
    const usr: Channel[] = [];
    for (const ch of channels) {
      if (SYSTEM_CHANNELS.includes(ch.name)) {
        sys.push(ch);
      } else {
        usr.push(ch);
      }
    }
    // Sort system channels in predefined order
    sys.sort((a, b) => SYSTEM_CHANNELS.indexOf(a.name) - SYSTEM_CHANNELS.indexOf(b.name));
    return { systemChannels: sys, userChannels: usr };
  }, [channels]);

  // Online member count
  const onlineCount = useMemo(
    () => members.filter(m => m.status !== 'offline').length,
    [members],
  );

  const renderChannel = (ch: Channel) => (
    <li
      key={ch.id}
      className={`channel-sidebar__item ${selectedChannel?.id === ch.id ? 'channel-sidebar__item--active' : ''}`}
      onClick={() => onSelect(ch)}
      role="button"
      tabIndex={0}
      onKeyDown={(e) => handleChannelKeyDown(ch, e)}
    >
      <span className="channel-sidebar__item-hash">#</span>
      <span className="channel-sidebar__item-name">{ch.name}</span>
    </li>
  );

  return (
    <>
      <aside className="channel-sidebar">
        <div className="channel-sidebar__header">
          <span className="channel-sidebar__title">Messenger</span>
          {onlineCount > 0 && (
            <span className="channel-sidebar__online-count">{onlineCount} online</span>
          )}
        </div>

        {loading && channels.length === 0 ? (
          <div style={{ padding: '8px 16px', color: 'var(--color-text-muted)' }}>Loading...</div>
        ) : (
          <>
            {systemChannels.length > 0 && (
              <div className="channel-sidebar__section">
                <span className="channel-sidebar__section-title">SYSTEM</span>
                <ul className="channel-sidebar__list">
                  {systemChannels.map(renderChannel)}
                </ul>
              </div>
            )}

            <div className="channel-sidebar__section">
              <div className="channel-sidebar__section-header">
                <span className="channel-sidebar__section-title">CHANNELS</span>
                <button
                  className="channel-sidebar__add-btn"
                  onClick={() => setShowModal(true)}
                  title="Create channel"
                >
                  +
                </button>
              </div>
              <ul className="channel-sidebar__list">
                {userChannels.length === 0 && systemChannels.length === 0 ? (
                  <li style={{ padding: '4px 16px', color: 'var(--color-text-muted)', fontSize: '12px' }}>
                    No channels
                  </li>
                ) : (
                  userChannels.map(renderChannel)
                )}
              </ul>
            </div>
          </>
        )}
      </aside>

      {showModal && (
        <div className="create-channel-modal" onClick={() => setShowModal(false)}>
          <div className="create-channel-modal__content" onClick={e => e.stopPropagation()}>
            <h3 className="create-channel-modal__title">Create Channel</h3>
            <div className="create-channel-modal__field">
              <label className="create-channel-modal__label">Name</label>
              <input
                className="create-channel-modal__input"
                value={newName}
                onChange={e => setNewName(e.target.value)}
                placeholder="e.g. general, design-review"
                autoFocus
              />
            </div>
            <div className="create-channel-modal__field">
              <label className="create-channel-modal__label">Description (optional)</label>
              <input
                className="create-channel-modal__input"
                value={newDesc}
                onChange={e => setNewDesc(e.target.value)}
                placeholder="What's this channel about?"
              />
            </div>
            <div className="create-channel-modal__actions">
              <button className="btn btn--secondary" onClick={() => setShowModal(false)}>
                Cancel
              </button>
              <button
                className="btn btn--primary"
                onClick={handleCreate}
                disabled={creating || !newName.trim()}
              >
                {creating ? 'Creating...' : 'Create'}
              </button>
            </div>
          </div>
        </div>
      )}
    </>
  );
}
