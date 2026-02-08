export type SortKey = 'date' | 'size' | 'name';
export type TimeFilter = 'all' | 'today' | 'week' | 'month';

interface FilterBarProps {
  sortBy: SortKey;
  onSortChange: (key: SortKey) => void;
  timeFilter: TimeFilter;
  onTimeFilterChange: (filter: TimeFilter) => void;
}

export function FilterBar({ sortBy, onSortChange, timeFilter, onTimeFilterChange }: FilterBarProps) {
  return (
    <div className="filter-bar">
      <div className="filter-bar__group">
        <label className="filter-bar__label">Sort</label>
        <select
          className="filter-bar__select"
          value={sortBy}
          onChange={e => onSortChange(e.target.value as SortKey)}
        >
          <option value="date">Date</option>
          <option value="size">Size</option>
          <option value="name">Name</option>
        </select>
      </div>
      <div className="filter-bar__group">
        <label className="filter-bar__label">Period</label>
        <div className="filter-bar__pills">
          {(['all', 'today', 'week', 'month'] as TimeFilter[]).map(f => (
            <button
              key={f}
              className={`filter-bar__pill ${timeFilter === f ? 'filter-bar__pill--active' : ''}`}
              onClick={() => onTimeFilterChange(f)}
            >
              {f === 'all' ? 'All' : f === 'today' ? 'Today' : f === 'week' ? '7d' : '30d'}
            </button>
          ))}
        </div>
      </div>
    </div>
  );
}
