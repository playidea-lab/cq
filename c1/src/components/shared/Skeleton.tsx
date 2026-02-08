interface SkeletonProps {
  variant?: 'line' | 'card' | 'list-item';
  count?: number;
  className?: string;
}

function SkeletonLine({ className }: { className?: string }) {
  return <div className={`skeleton skeleton--line ${className || ''}`} />;
}

function SkeletonCard({ className }: { className?: string }) {
  return (
    <div className={`skeleton skeleton--card ${className || ''}`}>
      <div className="skeleton__header" />
      <div className="skeleton__body">
        <div className="skeleton__line skeleton__line--full" />
        <div className="skeleton__line skeleton__line--medium" />
        <div className="skeleton__line skeleton__line--short" />
      </div>
    </div>
  );
}

function SkeletonListItem({ className }: { className?: string }) {
  return (
    <div className={`skeleton skeleton--list-item ${className || ''}`}>
      <div className="skeleton__line skeleton__line--full" />
      <div className="skeleton__line skeleton__line--medium" />
    </div>
  );
}

export function Skeleton({ variant = 'line', count = 1, className }: SkeletonProps) {
  const items = Array.from({ length: count }, (_, i) => i);

  return (
    <div className={`skeleton-container ${className || ''}`} aria-hidden="true">
      {items.map(i => {
        switch (variant) {
          case 'card':
            return <SkeletonCard key={i} />;
          case 'list-item':
            return <SkeletonListItem key={i} />;
          default:
            return <SkeletonLine key={i} />;
        }
      })}
    </div>
  );
}
