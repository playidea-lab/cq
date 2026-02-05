import '../styles/nodes.css';

const items = [
  { type: 'document', label: 'Document' },
  { type: 'config', label: 'Config' },
  { type: 'session', label: 'Session' },
  { type: 'task', label: 'Task' },
  { type: 'connection', label: 'Connection' },
];

export function Legend() {
  return (
    <div className="legend">
      <div className="legend__title">Node Types</div>
      <div className="legend__items">
        {items.map((item) => (
          <div key={item.type} className="legend__item">
            <div className={`legend__dot legend__dot--${item.type}`} />
            <span>{item.label}</span>
          </div>
        ))}
      </div>
    </div>
  );
}
