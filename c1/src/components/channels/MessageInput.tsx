import { useState, useCallback, useRef, useEffect, type KeyboardEvent } from 'react';
import type { C1Member } from '../../types';

interface MentionMeta {
  agent: string;
  task: string;
}

interface MessageInputProps {
  onSend: (content: string, metadata?: Record<string, unknown>) => Promise<unknown>;
  disabled?: boolean;
  placeholder?: string;
  agentMembers?: C1Member[];
}

interface MentionPopupProps {
  members: C1Member[];
  query: string;
  selectedIndex: number;
  onSelect: (member: C1Member) => void;
}

function MentionPopup({ members, query, selectedIndex, onSelect }: MentionPopupProps) {
  const filtered = members.filter(m =>
    m.display_name.toLowerCase().includes(query.toLowerCase())
  );

  if (filtered.length === 0) return null;

  return (
    <ul className="mention-popup" role="listbox" aria-label="Agent mentions">
      {filtered.map((m, i) => (
        <li
          key={m.id}
          className={`mention-popup__item${i === selectedIndex ? ' mention-popup__item--selected' : ''}`}
          role="option"
          aria-selected={i === selectedIndex}
          onMouseDown={e => {
            // prevent blur on textarea
            e.preventDefault();
            onSelect(m);
          }}
        >
          <span className="mention-popup__avatar">{m.avatar || m.display_name.charAt(0).toUpperCase()}</span>
          <span className="mention-popup__name">{m.display_name}</span>
          <span className="mention-popup__type">agent</span>
        </li>
      ))}
    </ul>
  );
}

function extractMentions(content: string): string[] {
  const matches = content.match(/@([\w\-. ]+)/g);
  if (!matches) return [];
  return matches.map(m => m.slice(1).trim());
}

export function MessageInput({
  onSend,
  disabled = false,
  placeholder,
  agentMembers = [],
}: MessageInputProps) {
  const [text, setText] = useState('');
  const [sending, setSending] = useState(false);
  const textareaRef = useRef<HTMLTextAreaElement>(null);

  // @mention popup state
  const [showPopup, setShowPopup] = useState(false);
  const [mentionQuery, setMentionQuery] = useState('');
  const [mentionIndex, setMentionIndex] = useState(0);
  const [mentionStart, setMentionStart] = useState(-1);

  // Filtered members for current query
  const filteredMembers = agentMembers.filter(m =>
    m.display_name.toLowerCase().includes(mentionQuery.toLowerCase())
  );

  // Detect @mention trigger on text change
  const handleChange = useCallback((e: React.ChangeEvent<HTMLTextAreaElement>) => {
    const value = e.target.value;
    const cursor = e.target.selectionStart ?? value.length;
    setText(value);

    // Auto-resize
    const el = textareaRef.current;
    if (el) {
      el.style.height = 'auto';
      el.style.height = Math.min(el.scrollHeight, 120) + 'px';
    }

    // Find the @ before the cursor
    const textBeforeCursor = value.slice(0, cursor);
    const atIdx = textBeforeCursor.lastIndexOf('@');
    if (atIdx !== -1) {
      // Check no whitespace between @ and cursor (in the query part)
      const queryPart = textBeforeCursor.slice(atIdx + 1);
      if (!queryPart.includes(' ') && !queryPart.includes('\n')) {
        setMentionStart(atIdx);
        setMentionQuery(queryPart);
        setMentionIndex(0);
        setShowPopup(agentMembers.length > 0);
        return;
      }
    }
    setShowPopup(false);
    setMentionStart(-1);
    setMentionQuery('');
  }, [agentMembers.length]);

  // Close popup if no matching members
  useEffect(() => {
    if (showPopup && filteredMembers.length === 0) {
      setShowPopup(false);
    }
  }, [showPopup, filteredMembers.length]);

  const selectMember = useCallback((member: C1Member) => {
    const el = textareaRef.current;
    const cursor = el?.selectionStart ?? text.length;
    // Replace @query with @display_name
    const before = text.slice(0, mentionStart);
    const after = text.slice(cursor);
    const newText = `${before}@${member.display_name} ${after}`;
    setText(newText);
    setShowPopup(false);
    setMentionStart(-1);
    setMentionQuery('');
    // Restore focus and move cursor after the inserted mention
    if (el) {
      el.focus();
      const newCursor = before.length + 1 + member.display_name.length + 1;
      requestAnimationFrame(() => {
        el.setSelectionRange(newCursor, newCursor);
        el.style.height = 'auto';
        el.style.height = Math.min(el.scrollHeight, 120) + 'px';
      });
    }
  }, [text, mentionStart]);

  const handleSend = useCallback(async () => {
    const trimmed = text.trim();
    if (!trimmed || sending) return;
    setSending(true);
    try {
      // Build metadata if message contains @mentions
      const mentionNames = extractMentions(trimmed);
      let metadata: Record<string, unknown> | undefined;
      if (mentionNames.length > 0) {
        // Use first mention as primary agent target
        const mentionMeta: MentionMeta = {
          agent: mentionNames[0],
          task: '',
        };
        metadata = { mention: mentionMeta };
      }
      await onSend(trimmed, metadata);
      setText('');
      setShowPopup(false);
      if (textareaRef.current) {
        textareaRef.current.style.height = 'auto';
      }
    } finally {
      setSending(false);
    }
  }, [text, sending, onSend]);

  const handleKeyDown = useCallback((e: KeyboardEvent<HTMLTextAreaElement>) => {
    if (showPopup && filteredMembers.length > 0) {
      if (e.key === 'ArrowDown') {
        e.preventDefault();
        setMentionIndex(i => Math.min(i + 1, filteredMembers.length - 1));
        return;
      }
      if (e.key === 'ArrowUp') {
        e.preventDefault();
        setMentionIndex(i => Math.max(i - 1, 0));
        return;
      }
      if (e.key === 'Enter') {
        e.preventDefault();
        selectMember(filteredMembers[mentionIndex]);
        return;
      }
      if (e.key === 'Escape') {
        e.preventDefault();
        setShowPopup(false);
        return;
      }
    }

    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      handleSend();
    }
  }, [showPopup, filteredMembers, mentionIndex, selectMember, handleSend]);

  const handleInput = useCallback(() => {
    const el = textareaRef.current;
    if (el) {
      el.style.height = 'auto';
      el.style.height = Math.min(el.scrollHeight, 120) + 'px';
    }
  }, []);

  return (
    <div className="message-input">
      <div className="message-input__wrapper">
        {showPopup && (
          <MentionPopup
            members={agentMembers}
            query={mentionQuery}
            selectedIndex={mentionIndex}
            onSelect={selectMember}
          />
        )}
        <textarea
          ref={textareaRef}
          className="message-input__textarea"
          value={text}
          onChange={handleChange}
          onKeyDown={handleKeyDown}
          onInput={handleInput}
          placeholder={placeholder ?? 'Type a message... (@mention to notify an agent)'}
          disabled={disabled || sending}
          rows={1}
          aria-label="Message input"
          aria-autocomplete={showPopup ? 'list' : 'none'}
        />
        <button
          className="message-input__send-btn"
          onClick={handleSend}
          disabled={disabled || sending || !text.trim()}
        >
          {sending ? 'Sending...' : 'Send'}
        </button>
      </div>
    </div>
  );
}
