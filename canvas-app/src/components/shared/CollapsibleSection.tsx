import { useState } from 'react';
import { ChevronRight, ChevronDown } from 'lucide-react';

interface CollapsibleSectionProps {
  title: string;
  defaultOpen?: boolean;
  className?: string;
  children: React.ReactNode;
}

export function CollapsibleSection({
  title,
  defaultOpen = false,
  className = '',
  children,
}: CollapsibleSectionProps) {
  const [open, setOpen] = useState(defaultOpen);

  return (
    <div className={`collapsible ${className}`}>
      <button
        className="collapsible__toggle"
        onClick={() => setOpen(prev => !prev)}
        aria-expanded={open}
      >
        {open ? <ChevronDown size={14} /> : <ChevronRight size={14} />}
        <span className="collapsible__title">{title}</span>
      </button>
      {open && <div className="collapsible__content">{children}</div>}
    </div>
  );
}
