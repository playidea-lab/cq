import { useState, useEffect } from 'react';
import { invoke } from '@tauri-apps/api/core';
import type { ProviderInfo } from '../../types';

const PROVIDER_COLORS: Record<string, string> = {
  claude_code: 'var(--color-accent-primary)',
  codex_cli: '#10a37f',
  cursor: '#7c3aed',
  gemini_cli: '#4285f4',
};

interface UsagePanelProps {
  projectPath: string;
}

export function UsagePanel({ projectPath }: UsagePanelProps) {
  const [providers, setProviders] = useState<ProviderInfo[]>([]);

  useEffect(() => {
    invoke<ProviderInfo[]>('list_providers', { path: projectPath })
      .then(setProviders)
      .catch(() => {});
  }, [projectPath]);

  if (providers.length === 0) return null;

  const maxCount = Math.max(...providers.map(p => p.session_count), 1);
  const totalSessions = providers.reduce((sum, p) => sum + p.session_count, 0);

  return (
    <div className="usage-panel">
      <div className="usage-panel__header">
        <h4 className="usage-panel__title">LLM Tools</h4>
        <span className="usage-panel__total">{totalSessions} sessions</span>
      </div>
      <div className="usage-panel__bars">
        {providers.map(p => (
          <div key={p.kind} className="usage-panel__row">
            <span
              className="usage-panel__icon"
              style={{ background: PROVIDER_COLORS[p.kind] || 'var(--color-text-muted)' }}
            >
              {p.icon}
            </span>
            <span className="usage-panel__name">{p.name}</span>
            <div className="usage-panel__bar-track">
              <div
                className="usage-panel__bar-fill"
                style={{
                  width: `${(p.session_count / maxCount) * 100}%`,
                  background: PROVIDER_COLORS[p.kind] || 'var(--color-text-muted)',
                }}
              />
            </div>
            <span className="usage-panel__count">{p.session_count}</span>
          </div>
        ))}
      </div>
    </div>
  );
}
