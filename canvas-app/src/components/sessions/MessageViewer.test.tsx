import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import { MessageViewer } from './MessageViewer';
import type { SessionMessage } from '../../types';

function makeMessage(overrides: Partial<SessionMessage> = {}): SessionMessage {
  return {
    msg_type: 'assistant',
    timestamp: '2026-02-08T10:00:00Z',
    uuid: 'test-uuid',
    content: [],
    ...overrides,
  };
}

describe('MessageViewer', () => {
  it('shows empty state when no messages', () => {
    render(<MessageViewer messages={[]} hasMore={false} loading={false} onLoadMore={vi.fn()} />);
    expect(screen.getByText('No messages')).toBeInTheDocument();
  });

  it('renders user message with "You" label', () => {
    render(
      <MessageViewer
        messages={[makeMessage({ msg_type: 'user', content: [{ block_type: 'text', text: 'Hello', tool_name: null, tool_input: null }] })]}
        hasMore={false}
        loading={false}
        onLoadMore={vi.fn()}
      />
    );
    expect(screen.getByText('You')).toBeInTheDocument();
    expect(screen.getByText('Hello')).toBeInTheDocument();
  });

  it('renders assistant message with "Assistant" label', () => {
    render(
      <MessageViewer
        messages={[makeMessage({ content: [{ block_type: 'text', text: 'Hi there', tool_name: null, tool_input: null }] })]}
        hasMore={false}
        loading={false}
        onLoadMore={vi.fn()}
      />
    );
    expect(screen.getByText('Assistant')).toBeInTheDocument();
    expect(screen.getByText('Hi there')).toBeInTheDocument();
  });

  it('renders tool_use block with tool name', () => {
    render(
      <MessageViewer
        messages={[makeMessage({
          content: [{
            block_type: 'tool_use',
            text: null,
            tool_name: 'Read',
            tool_input: { file_path: '/test.txt' },
          }],
        })]}
        hasMore={false}
        loading={false}
        onLoadMore={vi.fn()}
      />
    );
    expect(screen.getByText('Tool: Read')).toBeInTheDocument();
  });

  it('renders Load More button when hasMore is true', () => {
    render(
      <MessageViewer
        messages={[makeMessage()]}
        hasMore={true}
        loading={false}
        onLoadMore={vi.fn()}
      />
    );
    expect(screen.getByText('Load More')).toBeInTheDocument();
  });

  it('shows Loading... when loading', () => {
    render(
      <MessageViewer
        messages={[makeMessage()]}
        hasMore={true}
        loading={true}
        onLoadMore={vi.fn()}
      />
    );
    expect(screen.getByText('Loading...')).toBeInTheDocument();
  });
});
