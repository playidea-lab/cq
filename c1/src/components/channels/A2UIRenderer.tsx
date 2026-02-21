import { isA2UISpec } from '../../types/a2ui';

interface A2UIRendererProps {
  spec: unknown;
  onAction: (id: string, label: string) => void;
}

export function A2UIRenderer({ spec, onAction }: A2UIRendererProps) {
  if (!isA2UISpec(spec)) return null;

  return (
    <div className="a2ui-renderer">
      {spec.title && <div className="a2ui-renderer__title">{spec.title}</div>}
      <div className="a2ui-renderer__actions">
        {spec.items.map(action => (
          <button
            key={action.id}
            className={`a2ui-renderer__btn a2ui-renderer__btn--${action.style}`}
            onClick={() => onAction(action.id, action.label)}
          >
            {action.label}
          </button>
        ))}
      </div>
    </div>
  );
}
