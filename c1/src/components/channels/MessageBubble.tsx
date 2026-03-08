import { MarkdownViewer } from '../shared/MarkdownViewer';
import { A2UIRenderer } from './A2UIRenderer';
import { formatTime } from '../../utils/format';
import type { C1Message, C1Member, SenderType } from '../../types';

interface MessageBubbleProps {
  message: C1Message;
  member?: C1Member;
  onAction?: (id: string, label: string) => void;
}

function inferSenderType(msg: C1Message): SenderType {
  const meta = msg.metadata;
  if (meta && typeof meta === 'object' && 'sender_type' in meta) {
    return meta.sender_type as SenderType;
  }
  if (msg.participant_id.startsWith('system') || msg.participant_id === 'keeper') {
    return 'system';
  }
  if (msg.participant_id.startsWith('agent-') || msg.participant_id.startsWith('worker-')) {
    return 'agent';
  }
  return 'human';
}

function avatarContent(member?: C1Member, type?: SenderType): string {
  if (member) {
    switch (member.member_type) {
      case 'agent': return '\u{1F916}';  // robot
      case 'system': return '\u{2699}';  // gear
      case 'user': {
        const name = member.display_name || member.external_id;
        return name.charAt(0).toUpperCase();
      }
    }
  }
  switch (type) {
    case 'agent': return 'A';
    case 'system': return 'S';
    default: return 'H';
  }
}

function displayName(msg: C1Message, member?: C1Member, type?: SenderType): string {
  if (member?.display_name) return member.display_name;
  const meta = msg.metadata;
  if (meta && typeof meta === 'object' && 'display_name' in meta) {
    return meta.display_name as string;
  }
  switch (type) {
    case 'system': return 'System';
    default: return msg.participant_id;
  }
}

export function MessageBubble({ message, member, onAction }: MessageBubbleProps) {
  const type: SenderType = member
    ? (member.member_type === 'user' ? 'human' : member.member_type as SenderType)
    : inferSenderType(message);
  const avatarClass = type;

  const a2uiSpec = message.metadata?.a2ui;

  if (type === 'system' && !member) {
    return (
      <div className="message-bubble message-bubble--system">
        <div className="message-bubble__body">
          <div className="message-bubble__content">
            <MarkdownViewer content={message.content} />
          </div>
          {a2uiSpec != null && (
            <A2UIRenderer
              spec={a2uiSpec}
              onAction={onAction ?? (() => {})}
            />
          )}
        </div>
      </div>
    );
  }

  return (
    <div className="message-bubble">
      <div className={`message-bubble__avatar message-bubble__avatar--${avatarClass}`}>
        {avatarContent(member, type)}
      </div>
      <div className="message-bubble__body">
        <div className="message-bubble__header">
          <span className="message-bubble__sender">{displayName(message, member, type)}</span>
          <span className="message-bubble__time">{formatTime(message.created_at)}</span>
        </div>
        <div className="message-bubble__content">
          <MarkdownViewer content={message.content} />
        </div>
        {a2uiSpec != null && (
          <A2UIRenderer
            spec={a2uiSpec}
            onAction={onAction ?? (() => {})}
          />
        )}
      </div>
    </div>
  );
}
