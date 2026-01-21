/**
 * C4 API Client
 *
 * Provides type-safe API calls for the C4 Chat service with SSE streaming support.
 */

const API_BASE = process.env.NEXT_PUBLIC_API_URL || 'http://localhost:4000';

// =============================================================================
// Core Types
// =============================================================================

export interface ToolCallInfo {
  name: string;
  input: Record<string, unknown>;
  result?: string;
  success?: boolean;
  duration_ms?: number;
}

export interface Message {
  id: string;
  role: 'user' | 'assistant' | 'system' | 'tool';
  content: string;
  timestamp: string;
  metadata?: Record<string, unknown>;
  tool_calls?: ToolCallInfo[];
}

export interface ChatResponse {
  id: string;
  conversation_id: string;
  workspace_id?: string;
  message: Message;
  done: boolean;
  usage?: { input_tokens: number; output_tokens: number };
}

// =============================================================================
// SSE Event Types
// =============================================================================

export type SSEEventType =
  | 'start'
  | 'thinking'
  | 'tool_call'
  | 'tool_result'
  | 'chunk'
  | 'done'
  | 'error';

export interface SSEStartEvent {
  event: 'start';
  data: {
    conversation_id: string;
  };
}

export interface SSEThinkingEvent {
  event: 'thinking';
  data: {
    status: string;
  };
}

export interface SSEToolCallEvent {
  event: 'tool_call';
  data: {
    name: string;
    input: Record<string, unknown>;
  };
}

export interface SSEToolResultEvent {
  event: 'tool_result';
  data: {
    name: string;
    result: string;
    success: boolean;
    duration_ms: number;
  };
}

export interface SSEChunkEvent {
  event: 'chunk';
  data: {
    content: string;
  };
}

export interface SSEDoneEvent {
  event: 'done';
  data: {
    conversation_id: string;
    message: Message;
    success: boolean;
    turns: number;
    total_tool_calls: number;
    done: true;
  };
}

export interface SSEErrorEvent {
  event: 'error';
  data: {
    error: string;
  };
}

export type SSEEvent =
  | SSEStartEvent
  | SSEThinkingEvent
  | SSEToolCallEvent
  | SSEToolResultEvent
  | SSEChunkEvent
  | SSEDoneEvent
  | SSEErrorEvent;

export interface Project {
  id: string;
  name: string;
  status: string;
  tasks_done: number;
  tasks_pending: number;
  created_at: string;
}

export interface ProjectDetail extends Project {
  description?: string;
  workers: Record<string, { state: string; task_id: string | null }>;
  recent_events: Array<{ type: string; timestamp: string; data: unknown }>;
}

/**
 * Send a chat message
 */
export async function sendMessage(
  message: string,
  conversationId?: string,
  stream = false
): Promise<ChatResponse> {
  const response = await fetch(`${API_BASE}/api/chat/message`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      message,
      conversation_id: conversationId,
      stream,
    }),
  });

  if (!response.ok) {
    throw new Error(`API error: ${response.status}`);
  }

  return response.json();
}

/**
 * Get conversation history
 */
export async function getHistory(conversationId: string): Promise<Message[]> {
  const response = await fetch(`${API_BASE}/api/chat/history/${conversationId}`);
  
  if (!response.ok) {
    throw new Error(`API error: ${response.status}`);
  }
  
  return response.json();
}

/**
 * Get C4 status
 */
export async function getStatus(): Promise<{
  initialized: boolean;
  status?: string;
  queue?: { pending: number; done: number };
}> {
  const response = await fetch(`${API_BASE}/api/status`);
  
  if (!response.ok) {
    throw new Error(`API error: ${response.status}`);
  }
  
  return response.json();
}

// =============================================================================
// SSE Streaming
// =============================================================================

export interface StreamOptions {
  message: string;
  conversationId?: string;
  workspaceId?: string;
  apiKey?: string;
  onEvent: (event: SSEEvent) => void;
  onError?: (error: Error) => void;
}

/**
 * Parse SSE lines into events
 */
function parseSSELine(line: string, eventType: string): SSEEvent | null {
  if (!line.startsWith('data:')) return null;

  try {
    const data = JSON.parse(line.slice(5).trim());
    return { event: eventType as SSEEventType, data } as SSEEvent;
  } catch {
    return null;
  }
}

/**
 * Stream chat response using SSE with full event type support
 *
 * Events:
 * - start: Response started (includes conversation_id)
 * - thinking: Agent is processing
 * - tool_call: Tool being executed (name, input)
 * - tool_result: Tool execution result (name, result, success, duration_ms)
 * - chunk: Text content chunk
 * - done: Response complete (message, turns, total_tool_calls)
 * - error: Error occurred
 */
export function streamMessage(options: StreamOptions): () => void {
  const { message, conversationId, workspaceId, apiKey, onEvent, onError } =
    options;
  const controller = new AbortController();

  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
  };

  if (apiKey) {
    headers['X-API-Key'] = apiKey;
  }

  fetch(`${API_BASE}/api/chat/message`, {
    method: 'POST',
    headers,
    body: JSON.stringify({
      message,
      conversation_id: conversationId,
      workspace_id: workspaceId,
      stream: true,
    }),
    signal: controller.signal,
  })
    .then(async (response) => {
      if (!response.ok) {
        const errorText = await response.text();
        throw new Error(`API error ${response.status}: ${errorText}`);
      }

      const reader = response.body?.getReader();
      if (!reader) throw new Error('No response body');

      const decoder = new TextDecoder();
      let buffer = '';
      let currentEventType = 'message';

      while (true) {
        const { done, value } = await reader.read();
        if (done) break;

        buffer += decoder.decode(value, { stream: true });
        const lines = buffer.split('\n');
        buffer = lines.pop() || '';

        for (const line of lines) {
          const trimmedLine = line.trim();

          // Parse event type line
          if (trimmedLine.startsWith('event:')) {
            currentEventType = trimmedLine.slice(6).trim();
            continue;
          }

          // Parse data line
          if (trimmedLine.startsWith('data:')) {
            const event = parseSSELine(trimmedLine, currentEventType);
            if (event) {
              onEvent(event);
            }
            // Reset event type after processing
            currentEventType = 'message';
          }
        }
      }
    })
    .catch((error) => {
      if (error.name !== 'AbortError') {
        if (onError) {
          onError(error);
        }
        // Also emit as error event
        onEvent({
          event: 'error',
          data: { error: error.message },
        });
      }
    });

  return () => controller.abort();
}

/**
 * Legacy streaming function for backwards compatibility
 * @deprecated Use streamMessage with options object instead
 */
export function streamMessageLegacy(
  message: string,
  conversationId?: string,
  onChunk?: (chunk: string) => void,
  onDone?: (response: ChatResponse) => void,
  onError?: (error: Error) => void
): () => void {
  return streamMessage({
    message,
    conversationId,
    onEvent: (event) => {
      if (event.event === 'chunk' && onChunk) {
        onChunk(event.data.content);
      }
      if (event.event === 'done' && onDone) {
        onDone({
          id: '',
          conversation_id: event.data.conversation_id,
          message: event.data.message,
          done: true,
        });
      }
    },
    onError,
  });
}
