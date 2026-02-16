import type { C1Member, MemberStatus } from '../../types';

interface MembersPanelProps {
  members: C1Member[];
}

function statusDot(status: MemberStatus): string {
  switch (status) {
    case 'online': return 'members-panel__dot--online';
    case 'working': return 'members-panel__dot--working';
    case 'idle': return 'members-panel__dot--idle';
    default: return 'members-panel__dot--offline';
  }
}

function memberAvatar(member: C1Member): string {
  switch (member.member_type) {
    case 'agent': return '\u{1F916}';
    case 'system': return '\u{2699}';
    default: {
      const name = member.display_name || member.external_id;
      return name.charAt(0).toUpperCase();
    }
  }
}

function groupMembers(members: C1Member[]) {
  const online: C1Member[] = [];
  const offline: C1Member[] = [];
  for (const m of members) {
    if (m.status === 'offline') {
      offline.push(m);
    } else {
      online.push(m);
    }
  }
  return { online, offline };
}

export function MembersPanel({ members }: MembersPanelProps) {
  const { online, offline } = groupMembers(members);

  const renderMember = (member: C1Member) => (
    <div key={member.id} className="members-panel__member">
      <div className="members-panel__avatar-wrap">
        <span className={`members-panel__avatar members-panel__avatar--${member.member_type}`}>
          {memberAvatar(member)}
        </span>
        <span className={`members-panel__dot ${statusDot(member.status)}`} />
      </div>
      <div className="members-panel__info">
        <span className="members-panel__name">
          {member.display_name || member.external_id}
        </span>
        {member.status_text && (
          <span className="members-panel__status-text">{member.status_text}</span>
        )}
      </div>
    </div>
  );

  return (
    <aside className="members-panel">
      <div className="members-panel__header">Members</div>

      {online.length > 0 && (
        <div className="members-panel__group">
          <span className="members-panel__group-title">
            ONLINE — {online.length}
          </span>
          {online.map(renderMember)}
        </div>
      )}

      {offline.length > 0 && (
        <div className="members-panel__group">
          <span className="members-panel__group-title">
            OFFLINE — {offline.length}
          </span>
          {offline.map(renderMember)}
        </div>
      )}

      {members.length === 0 && (
        <div className="members-panel__empty">No members yet</div>
      )}
    </aside>
  );
}
