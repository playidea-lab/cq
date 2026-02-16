import { useMemo } from 'react';
import type { GitCommit } from '../../types';

interface GitGraphProps {
  commits: GitCommit[];
}

interface LaneInfo {
  lane: number;
  connections: { fromLane: number; toLane: number; type: 'straight' | 'merge' | 'branch' }[];
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
    const connections: LaneInfo['connections'] = [];

    // Find if this commit is expected in any lane
    for (let i = 0; i < activeLanes.length; i++) {
      if (activeLanes[i] === commit.hash) {
        myLane = i;
        break;
      }
    }

    // If not found in any lane, allocate a new one
    if (myLane === -1) {
      // Find first empty lane
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
        connections.push({ fromLane: i, toLane: myLane, type: 'merge' });
        activeLanes[i] = null;
      }
    }

    // Process parents
    if (commit.parents.length > 0) {
      // First parent continues in the same lane
      activeLanes[myLane] = commit.parents[0];
      connections.push({ fromLane: myLane, toLane: myLane, type: 'straight' });

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
          // Allocate new lane for this parent
          const emptyIdx = activeLanes.indexOf(null);
          if (emptyIdx >= 0) {
            parentLane = emptyIdx;
          } else {
            parentLane = activeLanes.length;
            activeLanes.push(null);
          }
          activeLanes[parentLane] = parentHash;
        }
        connections.push({ fromLane: myLane, toLane: parentLane, type: 'branch' });
      }
    } else {
      // Root commit — free the lane
      activeLanes[myLane] = null;
    }

    result.set(commit.hash, { lane: myLane, connections });
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

export function GitGraph({ commits }: GitGraphProps) {
  const lanes = useMemo(() => computeLanes(commits), [commits]);
  const maxLane = useMemo(() => {
    let max = 0;
    for (const info of lanes.values()) {
      if (info.lane > max) max = info.lane;
      for (const conn of info.connections) {
        if (conn.fromLane > max) max = conn.fromLane;
        if (conn.toLane > max) max = conn.toLane;
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
                {/* Draw active lane lines (vertical) */}
                {info.connections.map((conn, ci) => {
                  const fromX = conn.fromLane * LANE_WIDTH + 10;
                  const toX = conn.toLane * LANE_WIDTH + 10;
                  const connColor = LANE_COLORS[conn.fromLane % LANE_COLORS.length];

                  if (conn.type === 'straight') {
                    return (
                      <line
                        key={ci}
                        x1={fromX} y1={0}
                        x2={fromX} y2={ROW_HEIGHT}
                        stroke={connColor}
                        strokeWidth={2}
                        opacity={0.6}
                      />
                    );
                  }
                  if (conn.type === 'merge') {
                    return (
                      <path
                        key={ci}
                        d={`M ${fromX} 0 C ${fromX} ${cy}, ${toX} ${cy}, ${toX} ${cy}`}
                        fill="none"
                        stroke={LANE_COLORS[conn.fromLane % LANE_COLORS.length]}
                        strokeWidth={2}
                        opacity={0.6}
                      />
                    );
                  }
                  // branch
                  return (
                    <path
                      key={ci}
                      d={`M ${fromX} ${cy} C ${fromX} ${ROW_HEIGHT}, ${toX} ${cy}, ${toX} ${ROW_HEIGHT}`}
                      fill="none"
                      stroke={LANE_COLORS[conn.toLane % LANE_COLORS.length]}
                      strokeWidth={2}
                      opacity={0.6}
                    />
                  );
                })}

                {/* Draw node */}
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
      </div>
    </div>
  );
}
