import { useMemo } from 'react';
import type { GitCommit } from '../../types';

interface GitGraphProps {
  commits: GitCommit[];
  hasMore?: boolean;
  onLoadMore?: () => void;
}

const LANE_COLORS = [
  'var(--color-accent-primary, #3b82f6)',
  'var(--color-success, #22c55e)',
  'var(--color-warning, #eab308)',
  'var(--color-purple, #a855f7)',
  'var(--color-cyan, #06b6d4)',
  'var(--color-error, #ef4444)',
];

const LANE_WIDTH = 20;
const NODE_RADIUS = 5;
const ROW_HEIGHT = 32;
const PAD_LEFT = 10;
const CORNER_R = 6;

/** Assign each commit to a lane (column index) for graph layout */
function computeLanes(commits: GitCommit[]): Map<string, number> {
  const result = new Map<string, number>();
  const activeLanes: (string | null)[] = [];

  for (const commit of commits) {
    let myLane = -1;

    for (let i = 0; i < activeLanes.length; i++) {
      if (activeLanes[i] === commit.hash) {
        if (myLane === -1) {
          myLane = i;
        } else {
          activeLanes[i] = null;
        }
      }
    }

    if (myLane === -1) {
      const empty = activeLanes.indexOf(null);
      myLane = empty >= 0 ? empty : activeLanes.length;
      if (myLane >= activeLanes.length) activeLanes.push(null);
    }

    result.set(commit.hash, myLane);

    if (commit.parents.length > 0) {
      activeLanes[myLane] = commit.parents[0];
      for (let p = 1; p < commit.parents.length; p++) {
        const ph = commit.parents[p];
        let found = false;
        for (let i = 0; i < activeLanes.length; i++) {
          if (activeLanes[i] === ph) { found = true; break; }
        }
        if (!found) {
          const empty = activeLanes.indexOf(null);
          const pl = empty >= 0 ? empty : activeLanes.length;
          if (pl >= activeLanes.length) activeLanes.push(null);
          activeLanes[pl] = ph;
        }
      }
    } else {
      activeLanes[myLane] = null;
    }
  }

  return result;
}

function formatDate(dateStr: string): string {
  try {
    const date = new Date(dateStr);
    const now = new Date();
    const diffMs = now.getTime() - date.getTime();
    const diffMins = Math.floor(diffMs / 60000);
    if (diffMins < 60) return `${diffMins}m ago`;
    const diffHours = Math.floor(diffMins / 60);
    if (diffHours < 24) return `${diffHours}h ago`;
    const diffDays = Math.floor(diffHours / 24);
    if (diffDays < 7) return `${diffDays}d ago`;
    return date.toLocaleDateString(undefined, { month: 'short', day: 'numeric' });
  } catch {
    return dateStr;
  }
}

function formatRef(ref: string): { label: string; type: 'head' | 'branch' | 'remote' | 'tag' } {
  if (ref.startsWith('HEAD -> ')) return { label: ref.replace('HEAD -> ', ''), type: 'head' };
  if (ref.startsWith('tag: ')) return { label: ref.replace('tag: ', ''), type: 'tag' };
  if (ref.includes('/')) return { label: ref, type: 'remote' };
  return { label: ref, type: 'branch' };
}

export function GitGraph({ commits, hasMore, onLoadMore }: GitGraphProps) {
  const lanes = useMemo(() => computeLanes(commits), [commits]);

  const maxLane = useMemo(() => {
    let max = 0;
    for (const lane of lanes.values()) {
      if (lane > max) max = lane;
    }
    return max;
  }, [lanes]);

  const commitRowMap = useMemo(() => {
    const map = new Map<string, number>();
    commits.forEach((c, i) => map.set(c.hash, i));
    return map;
  }, [commits]);

  const svgWidth = (maxLane + 1) * LANE_WIDTH + PAD_LEFT * 2;
  const totalHeight = commits.length * ROW_HEIGHT;

  if (commits.length === 0) return null;

  // Build edges (commit → parent connections)
  const edges: React.ReactNode[] = [];
  commits.forEach((commit, rowIdx) => {
    const myLane = lanes.get(commit.hash)!;
    const cx = myLane * LANE_WIDTH + PAD_LEFT;
    const cy = rowIdx * ROW_HEIGHT + ROW_HEIGHT / 2;

    commit.parents.forEach((parentHash, pi) => {
      const parentRowIdx = commitRowMap.get(parentHash);
      const parentLane = parentRowIdx !== undefined
        ? (lanes.get(parentHash) ?? myLane)
        : myLane;
      const px = parentLane * LANE_WIDTH + PAD_LEFT;
      const py = parentRowIdx !== undefined
        ? parentRowIdx * ROW_HEIGHT + ROW_HEIGHT / 2
        : totalHeight;

      // Color: first parent uses commit's lane, others use parent's lane
      const edgeLane = pi === 0 ? myLane : parentLane;
      const color = LANE_COLORS[edgeLane % LANE_COLORS.length];

      if (cx === px) {
        // Same lane: straight vertical line
        edges.push(
          <line key={`e-${rowIdx}-${pi}`}
            x1={cx} y1={cy} x2={px} y2={py}
            stroke={color} strokeWidth={2} />
        );
      } else {
        // Cross-lane: angular "]" shape — vertical down, rounded corner, horizontal to parent
        const dy = py - cy;
        const dx = Math.abs(px - cx);
        const r = Math.min(CORNER_R, dx, dy / 2);
        const dir = px > cx ? 1 : -1;
        const sweep = dir > 0 ? 1 : 0;

        edges.push(
          <path key={`e-${rowIdx}-${pi}`}
            d={`M ${cx} ${cy} L ${cx} ${py - r} A ${r} ${r} 0 0 ${sweep} ${cx + dir * r} ${py} L ${px} ${py}`}
            fill="none" stroke={color} strokeWidth={2} />
        );
      }
    });
  });

  return (
    <div className="git-graph">
      <h4 className="git-graph__title">Git Graph</h4>
      <div className="git-graph__container">
        <div className="git-graph__canvas">
          <svg
            className="git-graph__svg"
            width={svgWidth}
            height={totalHeight}
          >
            {edges}
            {commits.map((commit, rowIdx) => {
              const myLane = lanes.get(commit.hash)!;
              const cx = myLane * LANE_WIDTH + PAD_LEFT;
              const cy = rowIdx * ROW_HEIGHT + ROW_HEIGHT / 2;
              const color = LANE_COLORS[myLane % LANE_COLORS.length];
              const isMerge = commit.parents.length > 1;

              return isMerge ? (
                <rect key={`n-${rowIdx}`}
                  x={cx - NODE_RADIUS} y={cy - NODE_RADIUS}
                  width={NODE_RADIUS * 2} height={NODE_RADIUS * 2}
                  fill={color} transform={`rotate(45, ${cx}, ${cy})`} />
              ) : (
                <circle key={`n-${rowIdx}`} cx={cx} cy={cy} r={NODE_RADIUS} fill={color} />
              );
            })}
          </svg>

          <div className="git-graph__info-list">
            {commits.map((commit) => (
              <div key={commit.hash} className="git-graph__info">
                <div className="git-graph__message-row">
                  <span className="git-graph__message">{commit.message}</span>
                  {commit.refs.length > 0 && (
                    <span className="git-graph__refs">
                      {commit.refs.map((ref, ri) => {
                        const { label, type } = formatRef(ref);
                        return (
                          <span key={ri} className={`git-graph__ref git-graph__ref--${type}`}>
                            {label}
                          </span>
                        );
                      })}
                    </span>
                  )}
                </div>
                <div className="git-graph__meta">
                  <span className="git-graph__hash">{commit.shortHash}</span>
                  <span className="git-graph__author">{commit.author}</span>
                  <span className="git-graph__date">{formatDate(commit.date)}</span>
                </div>
              </div>
            ))}
          </div>
        </div>

        {hasMore && onLoadMore && (
          <button className="git-graph__load-more" onClick={onLoadMore}>
            Load more commits
          </button>
        )}
      </div>
    </div>
  );
}
