import { useEffect } from 'react';
import type { CanvasNode } from '../types';
import '../styles/nodes.css';

interface DetailPanelProps {
  node: CanvasNode;
  onClose: () => void;
}

const typeLabels: Record<string, string> = {
  document: 'Document',
  config: 'Configuration',
  session: 'Session',
  task: 'C4 Task',
  connection: 'Connection',
};

export function DetailPanel({ node, onClose }: DetailPanelProps) {
  // Keyboard dismiss with Escape key
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        onClose();
      }
    };
    document.addEventListener('keydown', handleKeyDown);
    return () => document.removeEventListener('keydown', handleKeyDown);
  }, [onClose]);
  const formatMetadata = (meta: Record<string, unknown>): string => {
    try {
      return JSON.stringify(meta, null, 2);
    } catch {
      return String(meta);
    }
  };

  const formatTime = (timestamp?: number): string => {
    if (!timestamp) return 'Unknown';
    return new Date(timestamp).toLocaleString();
  };

  return (
    <div className="detail-panel">
      <div className="detail-panel__header">
        <span className="detail-panel__title">{node.label}</span>
        <button className="detail-panel__close" onClick={onClose}>
          ×
        </button>
      </div>
      <div className="detail-panel__content">
        <div className="detail-panel__section">
          <div className="detail-panel__label">Type</div>
          <div className="detail-panel__value">{typeLabels[node.type] || node.type}</div>
        </div>

        {node.path && (
          <div className="detail-panel__section">
            <div className="detail-panel__label">Path</div>
            <div className="detail-panel__value">{node.path}</div>
          </div>
        )}

        <div className="detail-panel__section">
          <div className="detail-panel__label">Last Modified</div>
          <div className="detail-panel__value">{formatTime(node.timestamp)}</div>
        </div>

        {Object.keys(node.metadata).length > 0 && (
          <div className="detail-panel__section">
            <div className="detail-panel__label">Metadata</div>
            <pre className="detail-panel__metadata">
              {formatMetadata(node.metadata)}
            </pre>
          </div>
        )}
      </div>
    </div>
  );
}
