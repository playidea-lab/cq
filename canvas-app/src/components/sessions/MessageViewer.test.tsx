import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import { MessageViewer } from './MessageViewer';
import type { SessionMessage } from '../../types';

function makeMessage(overrides: Partial<SessionMessage> = {}): SessionMessage {
  return {
    msg_type: 'assistant',
    timestamp: '2026-02-08T10:00:00Z',
    uuid: 'test-uuid',
    content: [{ block_type: 'text', text: 'default text', tool_name: null, tool_input: null }],
    ...overrides,
  };
}

describe('MessageViewer', () => {
  it('shows empty state when no messages', () => {
    render(<MessageViewer messages={[]} hasMore={false} loading={false} onLoadMore={vi.fn()} />);
    expect(screen.getByText('No messages')).toBeInTheDocument();
  });

  it('renders user message with "You" label and left alignment', () => {
    const { container } = render(
      <MessageViewer
        messages={[makeMessage({ msg_type: 'user', content: [{ block_type: 'text', text: 'Hello', tool_name: null, tool_input: null }] })]}
        hasMore={false}
        loading={false}
        onLoadMore={vi.fn()}
      />
    );
    expect(screen.getByText('You')).toBeInTheDocument();
    expect(screen.getByText('Hello')).toBeInTheDocument();
    expect(container.querySelector('.msg-row--user')).toBeInTheDocument();
  });

  it('renders assistant message with "Assistant" label and right alignment', () => {
    const { container } = render(
      <MessageViewer
        messages={[makeMessage({ content: [{ block_type: 'text', text: 'Hi there', tool_name: null, tool_input: null }] })]}
        hasMore={false}
        loading={false}
        onLoadMore={vi.fn()}
      />
    );
    expect(screen.getByText('Assistant')).toBeInTheDocument();
    expect(screen.getByText('Hi there')).toBeInTheDocument();
    expect(container.querySelector('.msg-row--assistant')).toBeInTheDocument();
  });

  it('renders tool_use as ToolCard with file path', () => {
    const { container } = render(
      <MessageViewer
        messages={[makeMessage({
          content: [{
            block_type: 'tool_use',
            text: null,
            tool_name: 'Read',
            tool_input: { file_path: '/Users/foo/git/c4/src/App.tsx' },
          }],
        })]}
        hasMore={false}
        loading={false}
        onLoadMore={vi.fn()}
      />
    );
    // ToolCard shows label and shortened path
    expect(screen.getByText('Read')).toBeInTheDocument();
    expect(container.querySelector('.tool-card')).toBeInTheDocument();
    expect(screen.getByText('.../c4/src/App.tsx')).toBeInTheDocument();
  });

  it('renders Bash tool_use with command', () => {
    render(
      <MessageViewer
        messages={[makeMessage({
          content: [{
            block_type: 'tool_use',
            text: null,
            tool_name: 'Bash',
            tool_input: { command: 'npm test' },
          }],
        })]}
        hasMore={false}
        loading={false}
        onLoadMore={vi.fn()}
      />
    );
    expect(screen.getByText('Bash')).toBeInTheDocument();
    expect(screen.getByText('npm test')).toBeInTheDocument();
  });

  it('renders tool_result with preview', () => {
    const { container } = render(
      <MessageViewer
        messages={[makeMessage({
          msg_type: 'user',
          content: [{
            block_type: 'tool_result',
            text: 'line 1\nline 2\nline 3\nline 4\nline 5\nline 6',
            tool_name: 'some-id',
            tool_input: null,
          }],
        })]}
        hasMore={false}
        loading={false}
        onLoadMore={vi.fn()}
      />
    );
    expect(container.querySelector('.tool-card--result')).toBeInTheDocument();
    expect(screen.getByText('Result')).toBeInTheDocument();
    // Preview shows first 4 lines
    expect(container.querySelector('.tool-card__preview')).toBeInTheDocument();
  });

  it('renders tool_use as collapsible when message also has text', () => {
    render(
      <MessageViewer
        messages={[makeMessage({
          content: [
            { block_type: 'text', text: 'Let me check', tool_name: null, tool_input: null },
            { block_type: 'tool_use', text: null, tool_name: 'Read', tool_input: { file_path: '/test.txt' } },
          ],
        })]}
        hasMore={false}
        loading={false}
        onLoadMore={vi.fn()}
      />
    );
    expect(screen.getByText('Let me check')).toBeInTheDocument();
    expect(screen.getByText('Read')).toBeInTheDocument();
  });

  it('skips messages with empty content', () => {
    const { container } = render(
      <MessageViewer
        messages={[makeMessage({ content: [] })]}
        hasMore={false}
        loading={false}
        onLoadMore={vi.fn()}
      />
    );
    expect(container.querySelector('.msg')).not.toBeInTheDocument();
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
