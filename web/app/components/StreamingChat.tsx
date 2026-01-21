'use client';

import { useState, useRef, useEffect, useCallback } from 'react';
import {
  streamMessage,
  Message,
  SSEEvent,
} from '@/lib/api';

// =============================================================================
// Types
// =============================================================================

interface PendingToolCall {
  id: string;
  name: string;
  input: Record<string, unknown>;
  status: 'running' | 'success' | 'error';
  result?: string;
  duration_ms?: number;
}

interface StreamingMessage extends Message {
  isStreaming?: boolean;
  pendingToolCalls?: PendingToolCall[];
}

interface ChatState {
  messages: StreamingMessage[];
  isStreaming: boolean;
  currentThinking: string | null;
  conversationId: string | null;
}

interface StreamingChatProps {
  conversationId?: string;
  workspaceId?: string;
  apiKey?: string;
  onConversationStart?: (id: string) => void;
}

// =============================================================================
// Tool Call Display Component
// =============================================================================

function ToolCallCard({ tool, isExpanded, onToggle }: {
  tool: PendingToolCall;
  isExpanded: boolean;
  onToggle: () => void;
}) {
  const getToolIcon = (name: string) => {
    const icons: Record<string, string> = {
      read_file: '\ud83d\udcc4',
      write_file: '\u270f\ufe0f',
      run_shell: '\ud83d\udcbb',
      search_files: '\ud83d\udd0d',
      list_directory: '\ud83d\udcc1',
      git_commit: '\ud83d\udcbe',
    };
    return icons[name] || '\ud83d\udd27';
  };

  const getStatusColor = (status: string) => {
    switch (status) {
      case 'running': return 'text-yellow-400';
      case 'success': return 'text-green-400';
      case 'error': return 'text-red-400';
      default: return 'text-gray-400';
    }
  };

  return (
    <div className="bg-gray-800 rounded-lg border border-gray-700 overflow-hidden my-2">
      <button
        onClick={onToggle}
        className="w-full px-3 py-2 flex items-center justify-between hover:bg-gray-750 transition-colors"
      >
        <div className="flex items-center gap-2">
          <span>{getToolIcon(tool.name)}</span>
          <span className="font-mono text-sm text-gray-200">{tool.name}</span>
          {tool.duration_ms !== undefined && (
            <span className="text-xs text-gray-500">
              ({tool.duration_ms}ms)
            </span>
          )}
        </div>
        <div className="flex items-center gap-2">
          <span className={`text-sm ${getStatusColor(tool.status)}`}>
            {tool.status === 'running' ? (
              <span className="inline-flex items-center">
                <span className="w-2 h-2 bg-yellow-400 rounded-full animate-pulse mr-1" />
                Running
              </span>
            ) : tool.status === 'success' ? (
              '\u2713 Done'
            ) : (
              '\u2717 Failed'
            )}
          </span>
          <span className="text-gray-500">{isExpanded ? '\u25bc' : '\u25b6'}</span>
        </div>
      </button>

      {isExpanded && (
        <div className="px-3 py-2 border-t border-gray-700 space-y-2">
          <div>
            <p className="text-xs text-gray-500 mb-1">Input:</p>
            <pre className="text-xs bg-gray-900 p-2 rounded overflow-x-auto text-gray-300">
              {JSON.stringify(tool.input, null, 2)}
            </pre>
          </div>
          {tool.result && (
            <div>
              <p className="text-xs text-gray-500 mb-1">Result:</p>
              <pre className="text-xs bg-gray-900 p-2 rounded overflow-x-auto max-h-40 text-gray-300">
                {tool.result.length > 500
                  ? tool.result.slice(0, 500) + '...(truncated)'
                  : tool.result}
              </pre>
            </div>
          )}
        </div>
      )}
    </div>
  );
}

// =============================================================================
// Thinking Indicator Component
// =============================================================================

function ThinkingIndicator({ status }: { status: string }) {
  return (
    <div className="flex items-center gap-2 text-gray-400 my-2">
      <div className="flex space-x-1">
        <div className="w-2 h-2 bg-blue-400 rounded-full animate-bounce" />
        <div className="w-2 h-2 bg-blue-400 rounded-full animate-bounce [animation-delay:0.1s]" />
        <div className="w-2 h-2 bg-blue-400 rounded-full animate-bounce [animation-delay:0.2s]" />
      </div>
      <span className="text-sm italic">{status || 'Thinking...'}</span>
    </div>
  );
}

// =============================================================================
// Message Bubble Component
// =============================================================================

function MessageBubble({ message, expandedTools, onToggleTool }: {
  message: StreamingMessage;
  expandedTools: Set<string>;
  onToggleTool: (id: string) => void;
}) {
  const isUser = message.role === 'user';

  return (
    <div className={`flex ${isUser ? 'justify-end' : 'justify-start'}`}>
      <div
        className={`max-w-[85%] rounded-lg ${
          isUser
            ? 'bg-blue-600 text-white px-4 py-2'
            : 'bg-gray-800 text-gray-100 px-4 py-3'
        }`}
      >
        {/* Tool Calls */}
        {message.pendingToolCalls && message.pendingToolCalls.length > 0 && (
          <div className="mb-2">
            {message.pendingToolCalls.map((tool) => (
              <ToolCallCard
                key={tool.id}
                tool={tool}
                isExpanded={expandedTools.has(tool.id)}
                onToggle={() => onToggleTool(tool.id)}
              />
            ))}
          </div>
        )}

        {/* Tool Calls from completed message */}
        {message.tool_calls && message.tool_calls.length > 0 && !message.pendingToolCalls && (
          <div className="mb-2">
            {message.tool_calls.map((tool, idx) => (
              <ToolCallCard
                key={`${message.id}-tool-${idx}`}
                tool={{
                  id: `${message.id}-tool-${idx}`,
                  name: tool.name,
                  input: tool.input,
                  status: tool.success === false ? 'error' : 'success',
                  result: tool.result,
                  duration_ms: tool.duration_ms,
                }}
                isExpanded={expandedTools.has(`${message.id}-tool-${idx}`)}
                onToggle={() => onToggleTool(`${message.id}-tool-${idx}`)}
              />
            ))}
          </div>
        )}

        {/* Message Content */}
        {message.content && (
          <p className="whitespace-pre-wrap">{message.content}</p>
        )}

        {/* Streaming cursor */}
        {message.isStreaming && (
          <span className="inline-block w-2 h-4 bg-gray-400 animate-pulse ml-1" />
        )}

        {/* Timestamp */}
        <p className={`text-xs mt-1 ${isUser ? 'text-blue-200' : 'text-gray-500'}`}>
          {new Date(message.timestamp).toLocaleTimeString()}
        </p>
      </div>
    </div>
  );
}

// =============================================================================
// Main StreamingChat Component
// =============================================================================

export default function StreamingChat({
  conversationId: initialConvId,
  workspaceId,
  apiKey,
  onConversationStart,
}: StreamingChatProps) {
  const [state, setState] = useState<ChatState>({
    messages: [],
    isStreaming: false,
    currentThinking: null,
    conversationId: initialConvId || null,
  });
  const [input, setInput] = useState('');
  const [expandedTools, setExpandedTools] = useState<Set<string>>(new Set());
  const messagesEndRef = useRef<HTMLDivElement>(null);
  const abortRef = useRef<(() => void) | null>(null);

  const scrollToBottom = useCallback(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' });
  }, []);

  useEffect(() => {
    scrollToBottom();
  }, [state.messages, state.currentThinking, scrollToBottom]);

  const toggleToolExpanded = useCallback((toolId: string) => {
    setExpandedTools((prev) => {
      const next = new Set(prev);
      if (next.has(toolId)) {
        next.delete(toolId);
      } else {
        next.add(toolId);
      }
      return next;
    });
  }, []);

  const handleEvent = useCallback((event: SSEEvent) => {
    switch (event.event) {
      case 'start':
        setState((prev) => {
          const newConvId = event.data.conversation_id;
          if (!prev.conversationId && newConvId) {
            onConversationStart?.(newConvId);
          }
          return {
            ...prev,
            conversationId: newConvId || prev.conversationId,
            isStreaming: true,
            currentThinking: null,
          };
        });
        break;

      case 'thinking':
        setState((prev) => ({
          ...prev,
          currentThinking: event.data.status,
        }));
        break;

      case 'tool_call':
        setState((prev) => {
          const messages = [...prev.messages];
          const lastMsg = messages[messages.length - 1];

          if (lastMsg && lastMsg.role === 'assistant') {
            const toolCall: PendingToolCall = {
              id: crypto.randomUUID(),
              name: event.data.name,
              input: event.data.input,
              status: 'running',
            };
            lastMsg.pendingToolCalls = [
              ...(lastMsg.pendingToolCalls || []),
              toolCall,
            ];
          }

          return {
            ...prev,
            messages,
            currentThinking: null,
          };
        });
        break;

      case 'tool_result':
        setState((prev) => {
          const messages = [...prev.messages];
          const lastMsg = messages[messages.length - 1];

          if (lastMsg && lastMsg.pendingToolCalls) {
            // Find the matching running tool call
            const toolIdx = lastMsg.pendingToolCalls.findIndex(
              (t) => t.name === event.data.name && t.status === 'running'
            );
            if (toolIdx !== -1) {
              lastMsg.pendingToolCalls[toolIdx] = {
                ...lastMsg.pendingToolCalls[toolIdx],
                status: event.data.success ? 'success' : 'error',
                result: event.data.result,
                duration_ms: event.data.duration_ms,
              };
            }
          }

          return { ...prev, messages };
        });
        break;

      case 'chunk':
        setState((prev) => {
          const messages = [...prev.messages];
          const lastMsg = messages[messages.length - 1];

          if (lastMsg && lastMsg.role === 'assistant' && lastMsg.isStreaming) {
            lastMsg.content += event.data.content;
          } else {
            // Start new assistant message
            messages.push({
              id: crypto.randomUUID(),
              role: 'assistant',
              content: event.data.content,
              timestamp: new Date().toISOString(),
              isStreaming: true,
              pendingToolCalls: [],
            });
          }

          return {
            ...prev,
            messages,
            currentThinking: null,
          };
        });
        break;

      case 'done':
        setState((prev) => {
          const messages = [...prev.messages];
          const lastMsg = messages[messages.length - 1];

          if (lastMsg && lastMsg.role === 'assistant') {
            lastMsg.isStreaming = false;
            // Update with final message data
            if (event.data.message) {
              lastMsg.content = event.data.message.content;
              lastMsg.tool_calls = event.data.message.tool_calls;
            }
            // Clear pending tool calls (they're now in tool_calls)
            delete lastMsg.pendingToolCalls;
          }

          return {
            ...prev,
            messages,
            isStreaming: false,
            currentThinking: null,
          };
        });
        break;

      case 'error':
        setState((prev) => {
          const messages = [...prev.messages];
          messages.push({
            id: crypto.randomUUID(),
            role: 'assistant',
            content: `Error: ${event.data.error}`,
            timestamp: new Date().toISOString(),
          });

          return {
            ...prev,
            messages,
            isStreaming: false,
            currentThinking: null,
          };
        });
        break;
    }
  }, [onConversationStart]);

  const handleSubmit = useCallback((e: React.FormEvent) => {
    e.preventDefault();
    if (!input.trim() || state.isStreaming) return;

    const userMessage: StreamingMessage = {
      id: crypto.randomUUID(),
      role: 'user',
      content: input.trim(),
      timestamp: new Date().toISOString(),
    };

    setState((prev) => ({
      ...prev,
      messages: [...prev.messages, userMessage],
      isStreaming: true,
      currentThinking: 'Starting...',
    }));
    setInput('');

    // Start streaming
    abortRef.current = streamMessage({
      message: userMessage.content,
      conversationId: state.conversationId || undefined,
      workspaceId,
      apiKey,
      onEvent: handleEvent,
      onError: (error) => {
        console.error('Stream error:', error);
      },
    });
  }, [input, state.isStreaming, state.conversationId, workspaceId, apiKey, handleEvent]);

  const handleCancel = useCallback(() => {
    if (abortRef.current) {
      abortRef.current();
      abortRef.current = null;
      setState((prev) => ({
        ...prev,
        isStreaming: false,
        currentThinking: null,
      }));
    }
  }, []);

  return (
    <div className="flex flex-col h-full bg-gray-900">
      {/* Messages Area */}
      <div className="flex-1 overflow-y-auto p-4 space-y-4">
        {state.messages.length === 0 && !state.isStreaming && (
          <div className="text-center text-gray-500 mt-8">
            <p className="text-lg">Welcome to C4 Chat</p>
            <p className="text-sm mt-2">
              {workspaceId
                ? 'Ask questions or give commands. I can read files, write code, and run shell commands.'
                : 'Ask questions about your project or get help with tasks.'}
            </p>
            {workspaceId && (
              <p className="text-xs mt-4 text-gray-600">
                Workspace: {workspaceId}
              </p>
            )}
          </div>
        )}

        {state.messages.map((message) => (
          <MessageBubble
            key={message.id}
            message={message}
            expandedTools={expandedTools}
            onToggleTool={toggleToolExpanded}
          />
        ))}

        {/* Thinking Indicator */}
        {state.currentThinking && (
          <div className="flex justify-start">
            <div className="bg-gray-800 rounded-lg px-4 py-2">
              <ThinkingIndicator status={state.currentThinking} />
            </div>
          </div>
        )}

        <div ref={messagesEndRef} />
      </div>

      {/* Input Area */}
      <form onSubmit={handleSubmit} className="p-4 border-t border-gray-700">
        <div className="flex gap-2">
          <input
            type="text"
            value={input}
            onChange={(e) => setInput(e.target.value)}
            placeholder={workspaceId ? "Give a command or ask a question..." : "Type a message..."}
            className="flex-1 bg-gray-800 border border-gray-600 rounded-lg px-4 py-2 text-white placeholder-gray-400 focus:outline-none focus:border-blue-500"
            disabled={state.isStreaming}
          />
          {state.isStreaming ? (
            <button
              type="button"
              onClick={handleCancel}
              className="bg-red-600 text-white px-6 py-2 rounded-lg hover:bg-red-700 transition-colors"
            >
              Cancel
            </button>
          ) : (
            <button
              type="submit"
              disabled={!input.trim()}
              className="bg-blue-600 text-white px-6 py-2 rounded-lg hover:bg-blue-700 disabled:bg-gray-600 disabled:cursor-not-allowed transition-colors"
            >
              Send
            </button>
          )}
        </div>
      </form>
    </div>
  );
}
