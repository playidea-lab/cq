interface StatusBadgeProps {
  status: string;
  className?: string;
}

const STATUS_COLORS: Record<string, string> = {
  // Task statuses
  done: 'badge--green',
  in_progress: 'badge--blue',
  pending: 'badge--gray',
  blocked: 'badge--red',
  // Project statuses
  EXECUTE: 'badge--blue',
  COMPLETE: 'badge--green',
  HALTED: 'badge--orange',
  ERROR: 'badge--red',
  INIT: 'badge--gray',
  DISCOVERY: 'badge--purple',
  DESIGN: 'badge--purple',
  PLAN: 'badge--cyan',
  CHECKPOINT: 'badge--orange',
};

export function StatusBadge({ status, className = '' }: StatusBadgeProps) {
  const colorClass = STATUS_COLORS[status] || 'badge--gray';
  return (
    <span className={`badge ${colorClass} ${className}`}>
      {status.replace('_', ' ')}
    </span>
  );
}
