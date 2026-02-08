import { useState, useRef } from 'react';
import { useVirtualizer } from '@tanstack/react-virtual';
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
    // Codex CLI tools
    case 'exec_command':
    case 'shell':
      return { icon: '$', label: 'Shell', detail: truncate(input.cmd || input.command || '', 120) };
    case 'apply_patch':
      return { icon: 'P', label: 'Patch', detail: truncate(String(Object.values(input)[0] || ''), 80) };
    case 'update_plan':
      return { icon: 'P', label: 'Plan', detail: 'Update plan' };
    // Cursor tools
    case 'read_file':
      return { icon: 'R', label: 'Read', detail: shortenPath(input.target_file || input.targetFile || '') };
    case 'edit_file':
      return { icon: 'E', label: 'Edit', detail: shortenPath(input.target_file || input.targetFile || '') };
    case 'list_dir':
      return { icon: 'D', label: 'List', detail: input.relative_workspace_path || input.path || '' };
    case 'codebase_search':
      return { icon: 'S', label: 'Search', detail: input.query || '' };
    case 'grep_search':
      return { icon: 'G', label: 'Grep', detail: input.query || '' };
    case 'file_search':
      return { icon: 'F', label: 'Find', detail: input.query || '' };
    case 'run_terminal_command':
      return { icon: '$', label: 'Terminal', detail: truncate(input.command || '', 120) };
    default: {
      // Try to extract something meaningful
      const firstVal = Object.values(input).find(v => typeof v === 'string' && v.length < 120);
      return { icon: 'T', label: name, detail: firstVal ? truncate(String(firstVal), 80) : '' };
    }
  }
}

function shortenPath(p: string): string {
  const parts = p.split('/').filter(Boolean);
  if (parts.length <= 3) return p;
  return '.../' + parts.slice(-3).join('/');
}

function truncate(s: string, max: number): string {
  if (s.length <= max) return s;
  return s.slice(0, max) + '...';
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

function resultSummary(text: string): string {
  const lines = text.split('\n');
  for (const line of lines) {
    const trimmed = line.trim();
    if (trimmed && !['', '{', '}', '[', ']', '},', '],'].includes(trimmed)) {
      return truncate(trimmed, 80);
    }
  }
  return truncate(lines[0] || '', 80);
}

function ToolResultCard({ block }: { block: ContentBlock }) {
  const [expanded, setExpanded] = useState(false);

  if (!block.text) return null;

  const lineCount = block.text.split('\n').length;
  const byteLen = block.text.length;
  const sizeHint = byteLen > 1024 ? `${(byteLen / 1024).toFixed(1)}KB` : `${byteLen}B`;

  return (
    <div className="tool-card tool-card--result">
      <button className="tool-card__header" onClick={() => setExpanded(!expanded)}>
        <span className="tool-card__icon">&#x2190;</span>
        <span className="tool-card__label">Result</span>
        <span className="tool-card__detail">
          {resultSummary(block.text)}
        </span>
        <span className="tool-card__meta">{lineCount}L / {sizeHint}</span>
        <span className="tool-card__chevron">{expanded ? '\u25B4' : '\u25BE'}</span>
      </button>
      {expanded && (
        <pre className="tool-card__body">{block.text}</pre>
      )}
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

function isToolResultOnly(message: SessionMessage): boolean {
  return message.msg_type === 'user' &&
    message.content.length > 0 &&
    message.content.every(block => block.block_type === 'tool_result');
}

function MessageBubble({ message }: { message: SessionMessage }) {
  const toolResultOnly = isToolResultOnly(message);
  const isUser = message.msg_type === 'user' && !toolResultOnly;
  const isAssistant = message.msg_type === 'assistant' || toolResultOnly;
  const isSummary = message.msg_type === 'summary';
  const isSystem = message.msg_type === 'system';

  const isToolOnly = (message.msg_type === 'assistant' || toolResultOnly) &&
    !hasTextContent(message) && message.content.length > 0;

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
  const parentRef = useRef<HTMLDivElement>(null);

  const virtualizer = useVirtualizer({
    count: messages.length + (hasMore ? 1 : 0),
    getScrollElement: () => parentRef.current,
    estimateSize: () => 120,
    overscan: 5,
  });

  if (messages.length === 0 && !loading) {
    return <div className="msg-viewer__empty">No messages</div>;
  }

  return (
    <div ref={parentRef} className="msg-viewer msg-viewer--virtual">
      <div
        style={{
          height: `${virtualizer.getTotalSize()}px`,
          width: '100%',
          position: 'relative',
        }}
      >
        {virtualizer.getVirtualItems().map(virtualItem => {
          const isLoadMore = virtualItem.index === messages.length;

          if (isLoadMore) {
            return (
              <div
                key="load-more"
                style={{
                  position: 'absolute',
                  top: 0,
                  left: 0,
                  width: '100%',
                  transform: `translateY(${virtualItem.start}px)`,
                }}
              >
                <div className="msg-viewer__load-more">
                  <button
                    className="btn btn--secondary btn--sm"
                    onClick={onLoadMore}
                    disabled={loading}
                  >
                    {loading ? 'Loading...' : 'Load More'}
                  </button>
                </div>
              </div>
            );
          }

          const msg = messages[virtualItem.index];
          return (
            <div
              key={msg.uuid || virtualItem.index}
              data-index={virtualItem.index}
              ref={virtualizer.measureElement}
              style={{
                position: 'absolute',
                top: 0,
                left: 0,
                width: '100%',
                transform: `translateY(${virtualItem.start}px)`,
              }}
            >
              <MessageBubble message={msg} />
            </div>
          );
        })}
      </div>
    </div>
  );
}
