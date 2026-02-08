import { useState, useEffect, useCallback } from 'react';
import { invoke } from '@tauri-apps/api/core';

/** Map editor CLI names to display labels */
const EDITOR_LABELS: Record<string, string> = {
  code: 'VS Code',
  cursor: 'Cursor',
  zed: 'Zed',
};

export function useEditors() {
  const [editors, setEditors] = useState<string[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    let cancelled = false;
    invoke<string[]>('detect_editors')
      .then((result) => {
        if (!cancelled) setEditors(result);
      })
      .catch(() => {
        // Detection failed; editors stays empty
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, []);

  const openInEditor = useCallback(
    async (projectPath: string, editor?: string) => {
      const target = editor || editors[0];
      if (!target) return;
      await invoke('open_in_editor', { projectPath, editor: target });
    },
    [editors],
  );

  const getLabel = useCallback((editor: string) => {
    return EDITOR_LABELS[editor] || editor;
  }, []);

  return { editors, loading, openInEditor, getLabel };
}
