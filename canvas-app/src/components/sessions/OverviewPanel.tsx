import type { ProviderInfo, ProviderKind } from '../../types';

interface OverviewPanelProps {
  providers: ProviderInfo[];
  onSelectProvider: (kind: ProviderKind) => void;
}

const PROVIDER_COLORS: Record<string, string> = {
  claude_code: 'var(--color-accent-primary)',
  codex_cli: '#10a37f',
  cursor: '#7c3aed',
  gemini_cli: '#4285f4',
};

export function OverviewPanel({ providers, onSelectProvider }: OverviewPanelProps) {
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
              <span className="overview__card-unit">sessions</span>
            </div>
            <div className="overview__card-path" title={p.data_path}>
              {shortenDataPath(p.data_path)}
            </div>
          </button>
        ))}
      </div>
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
