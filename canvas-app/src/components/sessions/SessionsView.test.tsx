import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent, waitFor, act } from '@testing-library/react';
import { SessionsView } from './SessionsView';

// Mock @tauri-apps/api/core
vi.mock('@tauri-apps/api/core', () => ({
  invoke: vi.fn(),
}));

// Mock @tauri-apps/api/event
vi.mock('@tauri-apps/api/event', () => ({
  listen: vi.fn(() => Promise.resolve(() => {})),
}));

import { invoke } from '@tauri-apps/api/core';
const mockInvoke = vi.mocked(invoke);

const SESSION_FIXTURE = {
  id: 'abc12345-1234-5678-9012-abcdefabcdef',
  slug: '-test',
  title: 'Test session',
  path: '/tmp/test.jsonl',
  line_count: 0,
  file_size: 1024,
  timestamp: Date.now(),
  git_branch: 'main',
};

/** Default mock that handles common commands */
function setupDefaultMock(overrides: Record<string, unknown> = {}) {
  mockInvoke.mockImplementation(async (cmd: string) => {
    if (cmd in overrides) return overrides[cmd];

    switch (cmd) {
      case 'list_providers':
        return [{ kind: 'claude_code', name: 'Claude Code', icon: 'C', session_count: 1, data_path: '' }];
      case 'list_sessions_for_provider':
        return [SESSION_FIXTURE];
      case 'list_sessions':
        return [SESSION_FIXTURE];
      case 'detect_editors':
        return [];
      case 'watch_sessions':
        return null;
      case 'search_sessions':
        return [];
      default:
        return [];
    }
  });
}

describe('SessionsView content search', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.useFakeTimers({ shouldAdvanceTime: true });
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it('renders search input when sessions exist', async () => {
    setupDefaultMock();
    render(<SessionsView projectPath="/test/project" />);

    await waitFor(() => {
      expect(screen.getByPlaceholderText('Search sessions...')).toBeInTheDocument();
    });
  });

  it('does not trigger content search for short queries (2 chars or less)', async () => {
    setupDefaultMock();
    render(<SessionsView projectPath="/test/project" />);

    await waitFor(() => {
      expect(screen.getByPlaceholderText('Search sessions...')).toBeInTheDocument();
    });

    const input = screen.getByPlaceholderText('Search sessions...');
    fireEvent.change(input, { target: { value: 'ab' } });

    await act(async () => {
      vi.advanceTimersByTime(500);
    });

    // Should NOT have called search_sessions
    const searchCalls = mockInvoke.mock.calls.filter(
      (call) => call[0] === 'search_sessions'
    );
    expect(searchCalls).toHaveLength(0);
  });

  it('triggers content search when query is longer than 2 chars with debounce', async () => {
    setupDefaultMock({
      search_sessions: [
        {
          session_id: SESSION_FIXTURE.id,
          session_title: 'Test session',
          session_path: '/tmp/test.jsonl',
          line_number: 42,
          matched_text: 'found the search term',
          context: '...found the search term in context...',
        },
      ],
    });

    render(<SessionsView projectPath="/test/project" />);

    await waitFor(() => {
      expect(screen.getByPlaceholderText('Search sessions...')).toBeInTheDocument();
    });

    const input = screen.getByPlaceholderText('Search sessions...');
    fireEvent.change(input, { target: { value: 'search term' } });

    // Before debounce fires, search_sessions should not be called
    const earlySearchCalls = mockInvoke.mock.calls.filter(
      (call) => call[0] === 'search_sessions'
    );
    expect(earlySearchCalls).toHaveLength(0);

    // Advance past debounce
    await act(async () => {
      vi.advanceTimersByTime(350);
    });

    await waitFor(() => {
      const searchCalls = mockInvoke.mock.calls.filter(
        (call) => call[0] === 'search_sessions'
      );
      expect(searchCalls).toHaveLength(1);
    });
  });

  it('displays search results with matched text and line number', async () => {
    setupDefaultMock({
      search_sessions: [
        {
          session_id: SESSION_FIXTURE.id,
          session_title: 'Test session',
          session_path: '/tmp/test.jsonl',
          line_number: 42,
          matched_text: 'found the search term',
          context: '...found the search term in context...',
        },
      ],
    });

    render(<SessionsView projectPath="/test/project" />);

    await waitFor(() => {
      expect(screen.getByPlaceholderText('Search sessions...')).toBeInTheDocument();
    });

    const input = screen.getByPlaceholderText('Search sessions...');
    fireEvent.change(input, { target: { value: 'search term' } });

    await act(async () => {
      vi.advanceTimersByTime(350);
    });

    await waitFor(() => {
      // Should show context preview
      expect(screen.getByText(/found the search term in context/)).toBeInTheDocument();
    });

    // Should show line number
    expect(screen.getByText('L42')).toBeInTheDocument();
  });

  it('shows "No results found" when content search returns empty', async () => {
    setupDefaultMock({
      search_sessions: [],
    });

    render(<SessionsView projectPath="/test/project" />);

    await waitFor(() => {
      expect(screen.getByPlaceholderText('Search sessions...')).toBeInTheDocument();
    });

    const input = screen.getByPlaceholderText('Search sessions...');
    fireEvent.change(input, { target: { value: 'nonexistent' } });

    await act(async () => {
      vi.advanceTimersByTime(350);
    });

    await waitFor(() => {
      expect(screen.getByText('No results found')).toBeInTheDocument();
    });
  });

  it('clears search results when query is cleared', async () => {
    setupDefaultMock({
      search_sessions: [
        {
          session_id: SESSION_FIXTURE.id,
          session_title: 'Test session',
          session_path: '/tmp/test.jsonl',
          line_number: 10,
          matched_text: 'match',
          context: 'some match context',
        },
      ],
    });

    render(<SessionsView projectPath="/test/project" />);

    await waitFor(() => {
      expect(screen.getByPlaceholderText('Search sessions...')).toBeInTheDocument();
    });

    const input = screen.getByPlaceholderText('Search sessions...');

    // Type a search query
    fireEvent.change(input, { target: { value: 'match' } });
    await act(async () => {
      vi.advanceTimersByTime(350);
    });
    await waitFor(() => {
      expect(screen.getByText(/some match context/)).toBeInTheDocument();
    });

    // Clear the query
    fireEvent.change(input, { target: { value: '' } });
    await act(async () => {
      vi.advanceTimersByTime(350);
    });

    // Search results should be gone
    await waitFor(() => {
      expect(screen.queryByText(/some match context/)).not.toBeInTheDocument();
    });
  });
});
