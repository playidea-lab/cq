import { useMemo } from 'react';
import type { GitCommit } from '../../types';

interface GitGraphProps {
  commits: GitCommit[];
  hasMore?: boolean;
  onLoadMore?: () => void;
}

interface LaneInfo {
  lane: number;
  /** All lanes that are active (have a branch passing through) at this row */
  activeLanes: number[];
  /** Special connections: merge curves coming in, branch curves going out */
  mergeFrom: number[];   // lanes merging INTO this commit's lane
  branchTo: number[];    // lanes branching OUT from this commit's lane
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
const NODE_RADIUS = 4;
const ROW_HEIGHT = 32;

function computeLanes(commits: GitCommit[]): Map<string, LaneInfo> {
  const result = new Map<string, LaneInfo>();
  // Track active lanes: each slot holds the hash of the commit expected next in that lane
  const activeLanes: (string | null)[] = [];

  for (const commit of commits) {
    let myLane = -1;
    const mergeFrom: number[] = [];
    const branchTo: number[] = [];

    // Find if this commit is expected in any lane
    for (let i = 0; i < activeLanes.length; i++) {
      if (activeLanes[i] === commit.hash) {
        myLane = i;
        break;
      }
    }

    // If not found in any lane, allocate a new one
    if (myLane === -1) {
      const emptyIdx = activeLanes.indexOf(null);
      if (emptyIdx >= 0) {
        myLane = emptyIdx;
      } else {
        myLane = activeLanes.length;
        activeLanes.push(null);
      }
    }

    // Close any other lanes that were also pointing to this commit (merge convergence)
    for (let i = 0; i < activeLanes.length; i++) {
      if (i !== myLane && activeLanes[i] === commit.hash) {
        mergeFrom.push(i);
        activeLanes[i] = null;
      }
    }

    // Process parents
    if (commit.parents.length > 0) {
      // First parent continues in the same lane
      activeLanes[myLane] = commit.parents[0];

      // Additional parents get new lanes (merge branches)
      for (let p = 1; p < commit.parents.length; p++) {
        const parentHash = commit.parents[p];
        // Check if parent is already tracked in another lane
        let parentLane = -1;
        for (let i = 0; i < activeLanes.length; i++) {
          if (activeLanes[i] === parentHash) {
            parentLane = i;
            break;
          }
        }
        if (parentLane === -1) {
          const emptyIdx = activeLanes.indexOf(null);
          if (emptyIdx >= 0) {
            parentLane = emptyIdx;
          } else {
            parentLane = activeLanes.length;
            activeLanes.push(null);
          }
          activeLanes[parentLane] = parentHash;
        }
        branchTo.push(parentLane);
      }
    } else {
      // Root commit — free the lane
      activeLanes[myLane] = null;
    }

    // Snapshot all currently active lanes for this row
    const active: number[] = [];
    for (let i = 0; i < activeLanes.length; i++) {
      if (activeLanes[i] !== null) {
        active.push(i);
      }
    }

    result.set(commit.hash, { lane: myLane, activeLanes: active, mergeFrom, branchTo });
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
  if (ref.startsWith('HEAD -> ')) {
    return { label: ref.replace('HEAD -> ', ''), type: 'head' };
  }
  if (ref.startsWith('tag: ')) {
    return { label: ref.replace('tag: ', ''), type: 'tag' };
  }
  if (ref.includes('/')) {
    return { label: ref, type: 'remote' };
  }
  return { label: ref, type: 'branch' };
}

export function GitGraph({ commits, hasMore, onLoadMore }: GitGraphProps) {
  const lanes = useMemo(() => computeLanes(commits), [commits]);
  const maxLane = useMemo(() => {
    let max = 0;
    for (const info of lanes.values()) {
      if (info.lane > max) max = info.lane;
      for (const l of info.activeLanes) {
        if (l > max) max = l;
      }
      for (const l of info.mergeFrom) {
        if (l > max) max = l;
      }
      for (const l of info.branchTo) {
        if (l > max) max = l;
      }
    }
    return max;
  }, [lanes]);

  const svgWidth = (maxLane + 1) * LANE_WIDTH + 12;

  if (commits.length === 0) {
    return null;
  }

  return (
    <div className="git-graph">
      <h4 className="git-graph__title">Git Graph</h4>
      <div className="git-graph__container">
        {commits.map((commit) => {
          const info = lanes.get(commit.hash);
          if (!info) return null;
          const isMerge = commit.parents.length > 1;
          const cx = info.lane * LANE_WIDTH + 10;
          const cy = ROW_HEIGHT / 2;
          const color = LANE_COLORS[info.lane % LANE_COLORS.length];

          return (
            <div key={commit.hash} className="git-graph__row">
              <svg
                className="git-graph__svg"
                width={svgWidth}
                height={ROW_HEIGHT}
                style={{ minWidth: svgWidth }}
              >
                {/* 1. Draw pass-through vertical lines for ALL active lanes */}
                {info.activeLanes.map((laneIdx) => {
                  const lx = laneIdx * LANE_WIDTH + 10;
                  const laneColor = LANE_COLORS[laneIdx % LANE_COLORS.length];
                  return (
                    <line
                      key={`lane-${laneIdx}`}
                      x1={lx} y1={0}
                      x2={lx} y2={ROW_HEIGHT}
                      stroke={laneColor}
                      strokeWidth={2}
                      opacity={0.5}
                    />
                  );
                })}

                {/* 2. Draw merge curves (other lanes converging into this commit) */}
                {info.mergeFrom.map((fromLane, mi) => {
                  const fromX = fromLane * LANE_WIDTH + 10;
                  const toX = cx;
                  const mergeColor = LANE_COLORS[fromLane % LANE_COLORS.length];
                  return (
                    <path
                      key={`merge-${mi}`}
                      d={`M ${fromX} 0 C ${fromX} ${cy}, ${toX} ${cy}, ${toX} ${cy}`}
                      fill="none"
                      stroke={mergeColor}
                      strokeWidth={2}
                      opacity={0.7}
                    />
                  );
                })}

                {/* 3. Draw branch curves (this commit branching out to new lanes) */}
                {info.branchTo.map((toLane, bi) => {
                  const fromX = cx;
                  const toX = toLane * LANE_WIDTH + 10;
                  const branchColor = LANE_COLORS[toLane % LANE_COLORS.length];
                  return (
                    <path
                      key={`branch-${bi}`}
                      d={`M ${fromX} ${cy} C ${fromX} ${ROW_HEIGHT}, ${toX} ${cy}, ${toX} ${ROW_HEIGHT}`}
                      fill="none"
                      stroke={branchColor}
                      strokeWidth={2}
                      opacity={0.7}
                    />
                  );
                })}

                {/* 4. Draw the commit node on top */}
                {isMerge ? (
                  <rect
                    x={cx - NODE_RADIUS}
                    y={cy - NODE_RADIUS}
                    width={NODE_RADIUS * 2}
                    height={NODE_RADIUS * 2}
                    fill={color}
                    transform={`rotate(45, ${cx}, ${cy})`}
                  />
                ) : (
                  <circle cx={cx} cy={cy} r={NODE_RADIUS} fill={color} />
                )}
              </svg>

              <div className="git-graph__info">
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
            </div>
          );
        })}
        {hasMore && onLoadMore && (
          <button className="git-graph__load-more" onClick={onLoadMore}>
            Load more commits
          </button>
        )}
      </div>
    </div>
  );
}
