import '../styles/nodes.css';

const nodeItems = [
  { type: 'document', label: 'Document' },
  { type: 'config', label: 'Config' },
  { type: 'session', label: 'Session' },
  { type: 'task', label: 'Task' },
  { type: 'connection', label: 'Connection' },
];

const edgeItems = [
  { type: 'references', label: 'References', color: 'blue' },
  { type: 'creates', label: 'Creates', color: 'green' },
  { type: 'depends', label: 'Depends', color: 'red' },
  { type: 'applies', label: 'Applies', color: 'orange' },
  { type: 'mentions', label: 'Mentions', color: 'grey' },
];

export function Legend() {
  return (
    <div className="legend">
      <div className="legend__section">
        <div className="legend__title">Node Types</div>
        <div className="legend__items">
          {nodeItems.map((item) => (
            <div key={item.type} className="legend__item">
              <div className={`legend__dot legend__dot--${item.type}`} />
              <span>{item.label}</span>
            </div>
          ))}
        </div>
      </div>

      <div className="legend__section">
        <div className="legend__title">Edge Relations</div>
        <div className="legend__items">
          {edgeItems.map((item) => (
            <div key={item.type} className="legend__item">
              <div className={`legend__line legend__line--${item.color}`} />
              <span>{item.label}</span>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}
