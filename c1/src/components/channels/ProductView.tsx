import { useState } from 'react';
import { useChannelPins } from '../../hooks/useChannelPins';
import { MarkdownViewer } from '../shared/MarkdownViewer';

interface ProductViewProps {
  channelId: string;
}

export function ProductView({ channelId }: ProductViewProps) {
  const { pins, loading } = useChannelPins(channelId);
  const [selectedIndex, setSelectedIndex] = useState(0);

  if (loading) {
    return null;
  }

  if (!pins.length) {
    return null;
  }

  const selectedPin = pins[selectedIndex] ?? pins[0];

  return (
    <div className="product-view">
      <div className="product-view__header">
        <span className="product-view__title">Product</span>
        {pins.length > 1 && (
          <select
            className="product-view__version-select"
            value={selectedIndex}
            onChange={e => setSelectedIndex(Number(e.target.value))}
          >
            {pins.map((p, i) => (
              <option key={p.id} value={i}>
                v{pins.length - i} &mdash; {p.created_at.slice(0, 10)}
              </option>
            ))}
          </select>
        )}
      </div>
      <div className="product-view__content">
        <MarkdownViewer content={selectedPin.content} />
      </div>
    </div>
  );
}
