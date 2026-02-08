import type { SearchHit } from '../../types';

interface SearchResultsProps {
  results: SearchHit[];
  loading: boolean;
  onSelect: (hit: SearchHit) => void;
}

export function SearchResults({ results, loading, onSelect }: SearchResultsProps) {
  if (loading) {
    return (
      <div className="search-results">
        <div className="search-results__loading">Searching...</div>
      </div>
    );
  }

  if (results.length === 0) {
    return (
      <div className="search-results">
        <div className="search-results__empty">No results found</div>
      </div>
    );
  }

  return (
    <ul className="search-results">
      {results.map((hit, index) => (
        <li key={`${hit.session_id}-${hit.line_number}-${index}`}>
          <button
            className="search-hit"
            onClick={() => onSelect(hit)}
          >
            <div className="search-hit__session">
              {hit.session_title || hit.session_id.slice(0, 8)}
            </div>
            <div className="search-hit__context">{hit.context}</div>
            <div className="search-hit__meta">
              <span className="search-hit__line">L{hit.line_number}</span>
            </div>
          </button>
        </li>
      ))}
    </ul>
  );
}
