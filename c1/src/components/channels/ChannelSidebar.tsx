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

  // Group channels into system, worker, and user-created
  const { systemChannels, workerChannels, userChannels } = useMemo(() => {
    const sys: Channel[] = [];
    const wkr: Channel[] = [];
    const usr: Channel[] = [];
    for (const ch of channels) {
      if (SYSTEM_CHANNELS.includes(ch.name)) {
        sys.push(ch);
      } else if (ch.channel_type === 'worker') {
        wkr.push(ch);
      } else {
        usr.push(ch);
      }
    }
    // Sort system channels in predefined order
    sys.sort((a, b) => SYSTEM_CHANNELS.indexOf(a.name) - SYSTEM_CHANNELS.indexOf(b.name));
    return { systemChannels: sys, workerChannels: wkr, userChannels: usr };
  }, [channels]);

  // Online agent (worker) count
  const onlineAgentCount = useMemo(
    () => members.filter(m => m.member_type === 'agent' && (m.status === 'online' || m.status === 'working')).length,
    [members],
  );

  // Find worker member by channel name (worker-{id} → member external_id={id})
  const getWorkerStatus = useCallback((ch: Channel) => {
    const workerID = ch.name.replace(/^worker-/, '');
    const member = members.find(m => m.external_id === workerID && m.member_type === 'agent');
    return member?.status ?? 'offline';
  }, [members]);

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

  const renderWorkerChannel = (ch: Channel) => {
    // #cq is the shared dispatch channel — show online agent count badge
    if (ch.name === 'cq') {
      return (
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
          {onlineAgentCount > 0 ? (
            <span className="channel-sidebar__worker-badge channel-sidebar__worker-badge--online">
              {onlineAgentCount}
            </span>
          ) : (
            <span className="channel-sidebar__worker-badge channel-sidebar__worker-badge--offline">
              0
            </span>
          )}
        </li>
      );
    }
    // Per-worker channels: show individual status dot
    const status = getWorkerStatus(ch);
    return (
      <li
        key={ch.id}
        className={`channel-sidebar__item ${selectedChannel?.id === ch.id ? 'channel-sidebar__item--active' : ''}`}
        onClick={() => onSelect(ch)}
        role="button"
        tabIndex={0}
        onKeyDown={(e) => handleChannelKeyDown(ch, e)}
      >
        <span className={`channel-sidebar__status-dot channel-sidebar__status-dot--${status}`} />
        <span className="channel-sidebar__item-name">{ch.name}</span>
      </li>
    );
  };

  return (
    <>
      <aside className="channel-sidebar">
        <div className="channel-sidebar__header">
          <span className="channel-sidebar__title">Messenger</span>
          {onlineAgentCount > 0 && (
            <span className="channel-sidebar__online-count">{onlineAgentCount} worker{onlineAgentCount > 1 ? 's' : ''}</span>
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

            {workerChannels.length > 0 && (
              <div className="channel-sidebar__section">
                <span className="channel-sidebar__section-title">WORKERS</span>
                <ul className="channel-sidebar__list">
                  {workerChannels.map(renderWorkerChannel)}
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
