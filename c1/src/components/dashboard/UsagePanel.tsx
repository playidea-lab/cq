import { useState, useEffect } from 'react';
import { invoke } from '@tauri-apps/api/core';
import type { ProviderInfo, TokenUsage } from '../../types';

const PROVIDER_COLORS: Record<string, string> = {
  claude_code: 'var(--color-accent-primary)',
  codex_cli: '#10a37f',
  cursor: '#7c3aed',
  gemini_cli: '#4285f4',
};

function formatTokens(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`;
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}K`;
  return String(n);
}

interface UsagePanelProps {
  projectPath: string;
}

export function UsagePanel({ projectPath }: UsagePanelProps) {
  const [providers, setProviders] = useState<ProviderInfo[]>([]);
  const [tokenUsage, setTokenUsage] = useState<Record<string, TokenUsage>>({});

  useEffect(() => {
    invoke<ProviderInfo[]>('list_providers', { path: projectPath })
      .then(result => {
        setProviders(result);
        // Fetch token usage for each provider in parallel
        for (const p of result) {
          invoke<TokenUsage | null>('get_provider_token_usage', {
            path: projectPath,
            provider: p.kind,
          })
            .then(usage => {
              if (usage) {
                setTokenUsage(prev => ({ ...prev, [p.kind]: usage }));
              }
            })
            .catch(() => {});
        }
      })
      .catch(() => {});
  }, [projectPath]);

  if (providers.length === 0) return null;

  const maxCount = Math.max(...providers.map(p => p.session_count), 1);
  const totalSessions = providers.reduce((sum, p) => sum + p.session_count, 0);

  // Total tokens across all providers
  const totalInput = Object.values(tokenUsage).reduce(
    (sum, u) => sum + u.input_tokens + u.cache_read_tokens + u.cache_creation_tokens, 0
  );
  const totalOutput = Object.values(tokenUsage).reduce(
    (sum, u) => sum + u.output_tokens, 0
  );

  return (
    <div className="usage-panel">
      <div className="usage-panel__header">
        <h4 className="usage-panel__title">LLM Tools</h4>
        <span className="usage-panel__total">{totalSessions} sessions</span>
      </div>
      <div className="usage-panel__bars">
        {providers.map(p => {
          return (
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
              <span className="usage-panel__count">
                {p.session_count}
                {p.is_global ? '*' : ''}
              </span>
            </div>
          );
        })}
      </div>
      {(totalInput > 0 || totalOutput > 0) && (
        <div className="usage-panel__tokens">
          <div className="usage-panel__tokens-header">Token Usage</div>
          {providers.map(p => {
            const usage = tokenUsage[p.kind];
            if (!usage) return null;
            const totalIn = usage.input_tokens + usage.cache_read_tokens + usage.cache_creation_tokens;
            return (
              <div key={p.kind} className="usage-panel__token-row">
                <span
                  className="usage-panel__token-dot"
                  style={{ background: PROVIDER_COLORS[p.kind] || 'var(--color-text-muted)' }}
                />
                <span className="usage-panel__token-name">{p.name}</span>
                <span className="usage-panel__token-in" title={`Input: ${totalIn.toLocaleString()} (${usage.cache_read_tokens.toLocaleString()} cached)`}>
                  {formatTokens(totalIn)} in
                </span>
                <span className="usage-panel__token-out" title={`Output: ${usage.output_tokens.toLocaleString()}`}>
                  {formatTokens(usage.output_tokens)} out
                </span>
              </div>
            );
          })}
          <div className="usage-panel__token-total">
            <span className="usage-panel__token-total-label">Total</span>
            <span className="usage-panel__token-in">{formatTokens(totalInput)} in</span>
            <span className="usage-panel__token-out">{formatTokens(totalOutput)} out</span>
          </div>
        </div>
      )}
    </div>
  );
}
