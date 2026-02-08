import type { ProviderInfo, ProviderKind } from '../../types';

interface ProviderTabsProps {
  providers: ProviderInfo[];
  active: ProviderKind;
  onSelect: (kind: ProviderKind) => void;
}

export function ProviderTabs({ providers, active, onSelect }: ProviderTabsProps) {
  if (providers.length <= 1) return null;

  return (
    <div className="provider-tabs">
      {providers.map(p => (
        <button
          key={p.kind}
          className={`provider-tabs__tab ${p.kind === active ? 'provider-tabs__tab--active' : ''}`}
          onClick={() => onSelect(p.kind)}
          title={`${p.name} — ${p.session_count} sessions`}
        >
          <span className="provider-tabs__icon">{p.icon}</span>
          <span className="provider-tabs__name">{p.name}</span>
          <span className="provider-tabs__count">{p.session_count}</span>
        </button>
      ))}
    </div>
  );
}
