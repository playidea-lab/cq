import { useState, useCallback, useRef, type KeyboardEvent } from 'react';

interface MessageInputProps {
  onSend: (content: string) => Promise<unknown>;
  disabled?: boolean;
  placeholder?: string;
}

export function MessageInput({ onSend, disabled = false, placeholder }: MessageInputProps) {
  const [text, setText] = useState('');
  const [sending, setSending] = useState(false);
  const textareaRef = useRef<HTMLTextAreaElement>(null);

  const handleSend = useCallback(async () => {
    const trimmed = text.trim();
    if (!trimmed || sending) return;
    setSending(true);
    try {
      await onSend(trimmed);
      setText('');
      if (textareaRef.current) {
        textareaRef.current.style.height = 'auto';
      }
    } finally {
      setSending(false);
    }
  }, [text, sending, onSend]);

  const handleKeyDown = useCallback((e: KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      handleSend();
    }
  }, [handleSend]);

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
        <textarea
          ref={textareaRef}
          className="message-input__textarea"
          value={text}
          onChange={e => setText(e.target.value)}
          onKeyDown={handleKeyDown}
          onInput={handleInput}
          placeholder={placeholder ?? 'Type a message... (Enter to send, Shift+Enter for newline)'}
          disabled={disabled || sending}
          rows={1}
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
