import { useState, useEffect } from 'react';
import { invoke } from '@tauri-apps/api/core';
import type { ProviderInfo, ProviderKind, DayStats } from '../../types';

interface OverviewPanelProps {
  providers: ProviderInfo[];
  onSelectProvider: (kind: ProviderKind) => void;
  projectPath?: string;
}

const PROVIDER_COLORS: Record<string, string> = {
  claude_code: 'var(--color-accent-primary)',
  codex_cli: '#10a37f',
  cursor: '#7c3aed',
  gemini_cli: '#4285f4',
};

export function OverviewPanel({ providers, onSelectProvider, projectPath }: OverviewPanelProps) {
  const totalSessions = providers.reduce((sum, p) => sum + p.session_count, 0);

  return (
    <div className="overview">
      <div className="overview__summary">
        <span className="overview__total">{totalSessions}</span>
        <span className="overview__label">total sessions across {providers.length} tools</span>
      </div>
      <div className="overview__cards">
        {providers.map(p => (
          <button
            key={p.kind}
            className="overview__card"
            onClick={() => onSelectProvider(p.kind)}
            style={{ borderLeftColor: PROVIDER_COLORS[p.kind] || 'var(--color-border-default)' }}
          >
            <div className="overview__card-header">
              <span
                className="overview__card-icon"
                style={{ background: PROVIDER_COLORS[p.kind] || 'var(--color-text-muted)' }}
              >
                {p.icon}
              </span>
              <span className="overview__card-name">{p.name}</span>
            </div>
            <div className="overview__card-stats">
              <span className="overview__card-count">{p.session_count}</span>
              <span className="overview__card-unit">
                sessions{p.is_global ? ' (all projects)' : ''}
              </span>
            </div>
            {projectPath && (
              <Sparkline
                projectPath={projectPath}
                provider={p.kind}
                color={PROVIDER_COLORS[p.kind] || 'var(--color-text-muted)'}
              />
            )}
            <div className="overview__card-path" title={p.data_path}>
              {shortenDataPath(p.data_path)}
            </div>
          </button>
        ))}
      </div>
    </div>
  );
}

interface SparklineProps {
  projectPath: string;
  provider: ProviderKind;
  color: string;
}

function Sparkline({ projectPath, provider, color }: SparklineProps) {
  const [timeline, setTimeline] = useState<DayStats[] | null>(null);

  useEffect(() => {
    let cancelled = false;
    invoke<DayStats[]>('get_provider_timeline', {
      path: projectPath,
      provider,
      days: 7,
    })
      .then((result) => {
        if (!cancelled) setTimeline(result);
      })
      .catch(() => {
        // Silently fail - sparkline is non-critical
      });

    return () => {
      cancelled = true;
    };
  }, [projectPath, provider]);

  if (!timeline || timeline.length === 0) return null;

  const maxCount = Math.max(...timeline.map(d => d.session_count), 1);

  return (
    <div className="overview__sparkline" title="Last 7 days activity">
      {timeline.map((day) => (
        <div
          key={day.date}
          className="overview__sparkline-bar"
          style={{
            height: `${Math.max((day.session_count / maxCount) * 100, day.session_count > 0 ? 10 : 2)}%`,
            background: day.session_count > 0 ? color : 'var(--color-bg-tertiary)',
          }}
          title={`${day.date}: ${day.session_count} sessions`}
        />
      ))}
    </div>
  );
}

function shortenDataPath(path: string): string {
  const home = path.replace(/^\/Users\/[^/]+/, '~');
  if (home.length <= 50) return home;
  const parts = home.split('/');
  if (parts.length <= 3) return home;
  return parts[0] + '/.../' + parts.slice(-2).join('/');
}
