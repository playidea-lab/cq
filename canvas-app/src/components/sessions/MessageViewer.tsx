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
      return (
        <CollapsibleSection
          key={index}
          title={`Tool: ${block.tool_name || 'unknown'}`}
          className="msg__tool-use"
        >
          <pre className="msg__tool-input">
            {block.tool_input ? JSON.stringify(block.tool_input, null, 2) : '{}'}
          </pre>
        </CollapsibleSection>
      );

    case 'tool_result':
      return (
        <CollapsibleSection
          key={index}
          title="Tool Result"
          className="msg__tool-result"
        >
          <pre className="msg__tool-output">{block.text || ''}</pre>
        </CollapsibleSection>
      );

    default:
      return null;
  }
}

function MessageBubble({ message }: { message: SessionMessage }) {
  const isUser = message.msg_type === 'user';
  const isSummary = message.msg_type === 'summary';
  const isSystem = message.msg_type === 'system';

  const className = [
    'msg',
    isUser ? 'msg--user' : '',
    isSummary ? 'msg--summary' : '',
    isSystem ? 'msg--system' : '',
    !isUser && !isSummary && !isSystem ? 'msg--assistant' : '',
  ].filter(Boolean).join(' ');

  const label = isUser ? 'You' : isSummary ? 'Summary' : isSystem ? 'System' : 'Assistant';

  return (
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
