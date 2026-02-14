import { MarkdownViewer } from '../shared/MarkdownViewer';
import type { C1Message, SenderType } from '../../types';

interface MessageBubbleProps {
  message: C1Message;
}

function inferSenderType(msg: C1Message): SenderType {
  const meta = msg.metadata;
  if (meta && typeof meta === 'object' && 'sender_type' in meta) {
    return meta.sender_type as SenderType;
  }
  // Heuristic: participant_id patterns
  if (msg.participant_id.startsWith('system') || msg.participant_id === 'keeper') {
    return 'system';
  }
  if (msg.participant_id.startsWith('agent-') || msg.participant_id.startsWith('worker-')) {
    return 'agent';
  }
  return 'human';
}

function senderInitial(type: SenderType): string {
  switch (type) {
    case 'human': return 'H';
    case 'agent': return 'A';
    case 'system': return 'S';
  }
}

function formatTime(iso: string): string {
  try {
    const d = new Date(iso);
    return d.toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit' });
  } catch {
    return '';
  }
}

function senderName(msg: C1Message, type: SenderType): string {
  const meta = msg.metadata;
  if (meta && typeof meta === 'object' && 'display_name' in meta) {
    return meta.display_name as string;
  }
  switch (type) {
    case 'system': return 'System';
    case 'agent': return msg.participant_id;
    default: return msg.participant_id;
  }
}

export function MessageBubble({ message }: MessageBubbleProps) {
  const type = inferSenderType(message);

  if (type === 'system') {
    return (
      <div className="message-bubble message-bubble--system">
        <div className="message-bubble__body">
          <div className="message-bubble__content">
            <MarkdownViewer content={message.content} />
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className="message-bubble">
      <div className={`message-bubble__avatar message-bubble__avatar--${type}`}>
        {senderInitial(type)}
      </div>
      <div className="message-bubble__body">
        <div className="message-bubble__header">
          <span className="message-bubble__sender">{senderName(message, type)}</span>
          <span className="message-bubble__time">{formatTime(message.created_at)}</span>
        </div>
        <div className="message-bubble__content">
          <MarkdownViewer content={message.content} />
        </div>
      </div>
    </div>
  );
}
