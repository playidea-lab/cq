import { useState } from 'react';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import rehypeSanitize from 'rehype-sanitize';
import { CollapsibleSection } from '../shared/CollapsibleSection';
import type { SessionMessage, ContentBlock } from '../../types';

interface MessageViewerProps {
  messages: SessionMessage[];
  hasMore: boolean;
  loading: boolean;
  onLoadMore: () => void;
}

// --- Tool info extraction ---

interface ToolInfo {
  icon: string;
  label: string;
  detail: string;
}

function extractToolInfo(block: ContentBlock): ToolInfo {
  const name = block.tool_name || 'unknown';
  const input = block.tool_input as Record<string, any> | null;

  if (!input) return { icon: 'T', label: name, detail: '' };

  switch (name) {
    case 'Read':
      return { icon: 'R', label: 'Read', detail: shortenPath(input.file_path || '') };
    case 'Edit':
      return { icon: 'E', label: 'Edit', detail: shortenPath(input.file_path || '') };
    case 'Write':
      return { icon: 'W', label: 'Write', detail: shortenPath(input.file_path || '') };
    case 'Bash':
      return { icon: '$', label: 'Bash', detail: truncate(input.command || '', 120) };
    case 'Grep':
      return {
        icon: 'G',
        label: 'Grep',
        detail: `"${input.pattern || ''}"${input.path ? ' in ' + shortenPath(input.path) : ''}`,
      };
    case 'Glob':
      return { icon: 'G', label: 'Glob', detail: input.pattern || '' };
    case 'WebFetch':
      return { icon: 'W', label: 'Fetch', detail: truncate(input.url || '', 80) };
    case 'WebSearch':
      return { icon: 'S', label: 'Search', detail: input.query || '' };
    case 'Task':
      return { icon: 'A', label: 'Agent', detail: input.description || input.prompt?.slice(0, 80) || '' };
    case 'TodoWrite':
    case 'TaskCreate':
      return { icon: '+', label: name, detail: input.subject || '' };
    default:
      // Try to extract something meaningful
      const firstVal = Object.values(input).find(v => typeof v === 'string' && v.length < 120);
      return { icon: 'T', label: name, detail: firstVal ? truncate(String(firstVal), 80) : '' };
  }
}

function shortenPath(p: string): string {
  // Show last 2-3 segments: /Users/foo/git/c4/src/App.tsx → src/App.tsx
  const parts = p.split('/').filter(Boolean);
  if (parts.length <= 3) return p;
  return '.../' + parts.slice(-3).join('/');
}

function truncate(s: string, max: number): string {
  if (s.length <= max) return s;
  return s.slice(0, max) + '...';
}

function previewText(text: string | null | undefined, lines: number = 4): { preview: string; hasMore: boolean } {
  if (!text) return { preview: '', hasMore: false };
  const allLines = text.split('\n');
  if (allLines.length <= lines) return { preview: text, hasMore: false };
  return { preview: allLines.slice(0, lines).join('\n'), hasMore: true };
}

// --- Tool Card component ---

function ToolCard({ block }: { block: ContentBlock }) {
  const [expanded, setExpanded] = useState(false);
  const info = extractToolInfo(block);

  return (
    <div className="tool-card">
      <button className="tool-card__header" onClick={() => setExpanded(!expanded)}>
        <span className="tool-card__icon">{info.icon}</span>
        <span className="tool-card__label">{info.label}</span>
        {info.detail && <span className="tool-card__detail">{info.detail}</span>}
        <span className="tool-card__chevron">{expanded ? '\u25B4' : '\u25BE'}</span>
      </button>
      {expanded && block.tool_input != null && (
        <pre className="tool-card__body">
          {String(JSON.stringify(block.tool_input, null, 2))}
        </pre>
      )}
    </div>
  );
}

function ToolResultCard({ block }: { block: ContentBlock }) {
  const [expanded, setExpanded] = useState(false);
  const { preview, hasMore } = previewText(block.text, 4);

  if (!block.text) return null;

  return (
    <div className="tool-card tool-card--result">
      <button className="tool-card__header" onClick={() => setExpanded(!expanded)}>
        <span className="tool-card__icon">&#x2190;</span>
        <span className="tool-card__label">Result</span>
        <span className="tool-card__detail">
          {truncate(block.text.split('\n')[0] || '', 80)}
        </span>
        {(hasMore || expanded) && (
          <span className="tool-card__chevron">{expanded ? '\u25B4' : '\u25BE'}</span>
        )}
      </button>
      {expanded ? (
        <pre className="tool-card__body">{block.text}</pre>
      ) : hasMore ? (
        <pre className="tool-card__preview">{preview}</pre>
      ) : null}
    </div>
  );
}

// --- Block rendering ---

function renderBlock(block: ContentBlock, index: number) {
  switch (block.block_type) {
    case 'text':
      return (
        <div key={index} className="msg__text">
          <ReactMarkdown remarkPlugins={[remarkGfm]} rehypePlugins={[rehypeSanitize]}>
            {block.text || ''}
          </ReactMarkdown>
        </div>
      );

    case 'thinking':
      return (
        <CollapsibleSection key={index} title="Thinking" className="msg__thinking">
          <pre className="msg__thinking-content">{block.text}</pre>
        </CollapsibleSection>
      );

    case 'tool_use':
      return <ToolCard key={index} block={block} />;

    case 'tool_result':
      return <ToolResultCard key={index} block={block} />;

    default:
      return null;
  }
}

// --- Message helpers ---

function hasTextContent(message: SessionMessage): boolean {
  return message.content.some(
    block => block.block_type === 'text' && block.text && block.text.trim().length > 0
  );
}

function MessageBubble({ message }: { message: SessionMessage }) {
  const isUser = message.msg_type === 'user';
  const isAssistant = message.msg_type === 'assistant';
  const isSummary = message.msg_type === 'summary';
  const isSystem = message.msg_type === 'system';

  const isToolOnly = isAssistant && !hasTextContent(message) && message.content.length > 0;

  const className = [
    'msg',
    isUser ? 'msg--user' : '',
    isAssistant ? 'msg--assistant' : '',
    isSummary ? 'msg--summary' : '',
    isSystem ? 'msg--system' : '',
    isToolOnly ? 'msg--tool-only' : '',
  ].filter(Boolean).join(' ');

  const label = isUser ? 'You' : isSummary ? 'Summary' : isSystem ? 'System' : 'Assistant';

  if (message.content.length === 0) return null;

  return (
    <div className={`msg-row ${isUser ? 'msg-row--user' : ''} ${isAssistant ? 'msg-row--assistant' : ''}`}>
      <div className={className}>
        <div className="msg__header">
          <span className="msg__role">{label}</span>
          {message.timestamp && (
            <span className="msg__time">
              {new Date(message.timestamp).toLocaleTimeString([], {
                hour: '2-digit',
                minute: '2-digit',
              })}
            </span>
          )}
        </div>
        <div className="msg__body">
          {message.content.map((block, i) => renderBlock(block, i))}
        </div>
      </div>
    </div>
  );
}

export function MessageViewer({ messages, hasMore, loading, onLoadMore }: MessageViewerProps) {
  if (messages.length === 0 && !loading) {
    return <div className="msg-viewer__empty">No messages</div>;
  }

  return (
    <div className="msg-viewer">
      {messages.map((msg, i) => (
        <MessageBubble key={msg.uuid || i} message={msg} />
      ))}
      {hasMore && (
        <div className="msg-viewer__load-more">
          <button
            className="btn btn--secondary btn--sm"
            onClick={onLoadMore}
            disabled={loading}
          >
            {loading ? 'Loading...' : 'Load More'}
          </button>
        </div>
      )}
    </div>
  );
}
