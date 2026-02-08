import { describe, it, expect, vi, beforeEach } from 'vitest';
import { renderHook, waitFor, act } from '@testing-library/react';
import { useEditors } from './useEditors';

vi.mock('@tauri-apps/api/core', () => ({
  invoke: vi.fn(),
}));

import { invoke } from '@tauri-apps/api/core';
const mockInvoke = vi.mocked(invoke);

describe('useEditors', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('detects available editors on mount', async () => {
    mockInvoke.mockImplementation(async (cmd: string) => {
      if (cmd === 'detect_editors') return ['code', 'cursor'];
      return [];
    });

    const { result } = renderHook(() => useEditors());

    // Initially loading
    expect(result.current.loading).toBe(true);
    expect(result.current.editors).toEqual([]);

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    expect(result.current.editors).toEqual(['code', 'cursor']);
    expect(mockInvoke).toHaveBeenCalledWith('detect_editors');
  });

  it('handles detection failure gracefully', async () => {
    mockInvoke.mockRejectedValue(new Error('IPC failed'));

    const { result } = renderHook(() => useEditors());

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    expect(result.current.editors).toEqual([]);
  });

  it('returns empty editors when none are detected', async () => {
    mockInvoke.mockImplementation(async (cmd: string) => {
      if (cmd === 'detect_editors') return [];
      return [];
    });

    const { result } = renderHook(() => useEditors());

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    expect(result.current.editors).toEqual([]);
  });

  it('openInEditor invokes the correct Tauri command', async () => {
    mockInvoke.mockImplementation(async (cmd: string) => {
      if (cmd === 'detect_editors') return ['code'];
      return undefined;
    });

    const { result } = renderHook(() => useEditors());

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    await act(async () => {
      await result.current.openInEditor('/Users/test/project');
    });

    expect(mockInvoke).toHaveBeenCalledWith('open_in_editor', {
      projectPath: '/Users/test/project',
      editor: 'code',
    });
  });

  it('openInEditor allows overriding the editor', async () => {
    mockInvoke.mockImplementation(async (cmd: string) => {
      if (cmd === 'detect_editors') return ['code', 'cursor'];
      return undefined;
    });

    const { result } = renderHook(() => useEditors());

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    await act(async () => {
      await result.current.openInEditor('/Users/test/project', 'cursor');
    });

    expect(mockInvoke).toHaveBeenCalledWith('open_in_editor', {
      projectPath: '/Users/test/project',
      editor: 'cursor',
    });
  });

  it('openInEditor does nothing if no editors available', async () => {
    mockInvoke.mockImplementation(async (cmd: string) => {
      if (cmd === 'detect_editors') return [];
      return undefined;
    });

    const { result } = renderHook(() => useEditors());

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    await act(async () => {
      await result.current.openInEditor('/Users/test/project');
    });

    // Only detect_editors should have been called, not open_in_editor
    const openCalls = mockInvoke.mock.calls.filter(
      (call) => call[0] === 'open_in_editor'
    );
    expect(openCalls).toHaveLength(0);
  });

  it('getLabel maps editor CLI names to display names', async () => {
    mockInvoke.mockImplementation(async (cmd: string) => {
      if (cmd === 'detect_editors') return ['code'];
      return [];
    });

    const { result } = renderHook(() => useEditors());

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    expect(result.current.getLabel('code')).toBe('VS Code');
    expect(result.current.getLabel('cursor')).toBe('Cursor');
    expect(result.current.getLabel('zed')).toBe('Zed');
    expect(result.current.getLabel('unknown')).toBe('unknown');
  });
});
