import { useState } from 'react';
import { MarkdownViewer } from '../shared/MarkdownViewer';
import { formatTime } from '../../utils/format';
import type { C1Message, C1Member } from '../../types';

interface AgentThreadProps {
  messages: C1Message[];
  agentName: string;
  isComplete: boolean;
  defaultExpanded?: boolean;
  getMember?: (memberId: string) => C1Member | undefined;
}

export function AgentThread({ messages, agentName, isComplete, defaultExpanded, getMember }: AgentThreadProps) {
  // in-progress = auto-expand; completed/failed/cancelled = collapsed by default
  const initialExpanded = defaultExpanded ?? !isComplete;
  const [isExpanded, setIsExpanded] = useState(initialExpanded);

  const taskTitle = messages[0]?.content?.slice(0, 60) ?? 'Task';
  const statusIcon = isComplete ? '✅' : '🔄';

  if (!isExpanded) {
    return (
      <div
        className="agent-thread agent-thread--collapsed"
        onClick={() => setIsExpanded(true)}
        role="button"
        tabIndex={0}
        onKeyDown={(e) => { if (e.key === 'Enter' || e.key === ' ') setIsExpanded(true); }}
      >
        <span className="agent-thread__summary">
          🤖 <strong>{agentName}</strong> — {taskTitle}{messages[0]?.content && messages[0].content.length > 60 ? '…' : ''}{' '}
          <span className="agent-thread__count">({messages.length} messages)</span>{' '}
          {statusIcon}
        </span>
        <span className="agent-thread__expand-hint">▼</span>
      </div>
    );
  }

  return (
    <div className="agent-thread agent-thread--expanded">
      <div
        className="agent-thread__header"
        onClick={() => setIsExpanded(false)}
        role="button"
        tabIndex={0}
        onKeyDown={(e) => { if (e.key === 'Enter' || e.key === ' ') setIsExpanded(false); }}
      >
        <span className="agent-thread__header-title">
          🤖 <strong>{agentName}</strong> — {taskTitle}{messages[0]?.content && messages[0].content.length > 60 ? '…' : ''}
        </span>
        <span className="agent-thread__toggle">
          {statusIcon} ▲
        </span>
      </div>
      <div className="agent-thread__messages">
        {messages.map(msg => {
          const member = msg.member_id && getMember ? getMember(msg.member_id) : undefined;
          return (
            <div key={msg.id} className="agent-thread__message">
              <div className="agent-thread__message-meta">
                <span className="agent-thread__message-sender">
                  {member?.display_name ?? msg.participant_id}
                </span>
                <span className="agent-thread__message-time">
                  {formatTime(msg.created_at)}
                </span>
              </div>
              <div className="agent-thread__message-content">
                <MarkdownViewer content={msg.content} />
              </div>
            </div>
          );
        })}
      </div>
    </div>
  );
}
