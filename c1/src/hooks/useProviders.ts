import { useState, useCallback } from 'react';
import { invoke } from '@tauri-apps/api/core';
import type { ProviderInfo, ProviderKind } from '../types';

export function useProviders() {
  const [providers, setProviders] = useState<ProviderInfo[]>([]);
  const [activeProvider, setActiveProvider] = useState<ProviderKind>('claude_code');
  const [loading, setLoading] = useState(false);

  const loadProviders = useCallback(async (projectPath: string) => {
    setLoading(true);
    try {
      const result = await invoke<ProviderInfo[]>('list_providers', { path: projectPath });
      setProviders(result);
      // Default to first provider if current one isn't available
      if (result.length > 0 && !result.find(p => p.kind === activeProvider)) {
        setActiveProvider(result[0].kind);
      }
    } catch {
      // Fallback: at least show Claude Code
      setProviders([{
        kind: 'claude_code',
        name: 'Claude Code',
        icon: 'C',
        session_count: 0,
        data_path: '',
        is_global: false,
      }]);
    } finally {
      setLoading(false);
    }
  }, [activeProvider]);

  return {
    providers,
    activeProvider,
    setActiveProvider,
    loading,
    loadProviders,
  };
}
