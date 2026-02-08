import { useState, useEffect } from 'react';
import { invoke } from '@tauri-apps/api/core';
import type { ProviderKind, SessionStats } from '../../types';

interface AnalyticsPanelProps {
  sessionPath: string;
  provider: ProviderKind;
}

export function AnalyticsPanel({ sessionPath, provider }: AnalyticsPanelProps) {
  const [stats, setStats] = useState<SessionStats | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    setLoading(true);
    setError(null);

    invoke<SessionStats>('get_session_stats', { sessionPath, provider })
      .then((result) => {
        if (!cancelled) setStats(result);
      })
      .catch((err) => {
        if (!cancelled) setError(String(err));
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });

    return () => {
      cancelled = true;
    };
  }, [sessionPath, provider]);

  if (loading) {
    return <div className="analytics-panel__loading">Computing analytics...</div>;
  }

  if (error) {
    return <div className="analytics-panel__error">Failed to load analytics: {error}</div>;
  }

  if (!stats) {
    return <div className="analytics-panel__empty">No analytics data available</div>;
  }

  const totalTokens = stats.total_input_tokens + stats.total_output_tokens;
  const inputPct = totalTokens > 0 ? (stats.total_input_tokens / totalTokens) * 100 : 0;
  const outputPct = totalTokens > 0 ? (stats.total_output_tokens / totalTokens) * 100 : 0;
  const maxToolCount = stats.tool_calls.length > 0 ? stats.tool_calls[0].count : 1;

  return (
    <div className="analytics-panel">
      {/* Header stats row */}
      <div className="analytics-panel__header">
        <div className="analytics-panel__stat">
          <span className="analytics-panel__stat-value">{formatDuration(stats.duration_seconds)}</span>
          <span className="analytics-panel__stat-label">Duration</span>
        </div>
        <div className="analytics-panel__stat">
          <span className="analytics-panel__stat-value">{stats.total_messages}</span>
          <span className="analytics-panel__stat-label">Messages</span>
        </div>
        <div className="analytics-panel__stat">
          <span className="analytics-panel__stat-value">${stats.estimated_cost_usd.toFixed(2)}</span>
          <span className="analytics-panel__stat-label">Est. Cost</span>
        </div>
        <div className="analytics-panel__stat">
          <span className="analytics-panel__stat-value">{stats.files_changed}</span>
          <span className="analytics-panel__stat-label">Files Changed</span>
        </div>
      </div>

      {/* Message counts */}
      <div className="analytics-panel__section">
        <h4 className="analytics-panel__section-title">Messages</h4>
        <div className="analytics-panel__msg-counts">
          <div className="analytics-panel__msg-row">
            <span className="analytics-panel__msg-label">User</span>
            <div className="analytics-panel__bar-track">
              <div
                className="analytics-panel__bar-fill analytics-panel__bar-fill--user"
                style={{
                  width: stats.total_messages > 0
                    ? `${(stats.user_messages / stats.total_messages) * 100}%`
                    : '0%',
                }}
              />
            </div>
            <span className="analytics-panel__msg-count">{stats.user_messages}</span>
          </div>
          <div className="analytics-panel__msg-row">
            <span className="analytics-panel__msg-label">Assistant</span>
            <div className="analytics-panel__bar-track">
              <div
                className="analytics-panel__bar-fill analytics-panel__bar-fill--assistant"
                style={{
                  width: stats.total_messages > 0
                    ? `${(stats.assistant_messages / stats.total_messages) * 100}%`
                    : '0%',
                }}
              />
            </div>
            <span className="analytics-panel__msg-count">{stats.assistant_messages}</span>
          </div>
        </div>
      </div>

      {/* Token usage stacked bar */}
      <div className="analytics-panel__section">
        <h4 className="analytics-panel__section-title">Token Usage</h4>
        <div className="analytics-panel__token-bar">
          <div
            className="analytics-panel__token-segment analytics-panel__token-segment--input"
            style={{ width: `${inputPct}%` }}
            title={`Input: ${formatTokens(stats.total_input_tokens)}`}
          />
          <div
            className="analytics-panel__token-segment analytics-panel__token-segment--output"
            style={{ width: `${outputPct}%` }}
            title={`Output: ${formatTokens(stats.total_output_tokens)}`}
          />
        </div>
        <div className="analytics-panel__token-legend">
          <span className="analytics-panel__token-legend-item">
            <span className="analytics-panel__token-dot analytics-panel__token-dot--input" />
            Input: {formatTokens(stats.total_input_tokens)}
          </span>
          <span className="analytics-panel__token-legend-item">
            <span className="analytics-panel__token-dot analytics-panel__token-dot--output" />
            Output: {formatTokens(stats.total_output_tokens)}
          </span>
          {stats.cache_read_tokens > 0 && (
            <span className="analytics-panel__token-legend-item">
              <span className="analytics-panel__token-dot analytics-panel__token-dot--cache" />
              Cache: {formatTokens(stats.cache_read_tokens)}
            </span>
          )}
        </div>
      </div>

      {/* Tool usage top 10 */}
      {stats.tool_calls.length > 0 && (
        <div className="analytics-panel__section">
          <h4 className="analytics-panel__section-title">
            Tool Usage (top {stats.tool_calls.length})
          </h4>
          <div className="analytics-panel__tool-list">
            {stats.tool_calls.map((tool) => (
              <div key={tool.tool_name} className="analytics-panel__tool-row">
                <span className="analytics-panel__tool-name">{tool.tool_name}</span>
                <div className="analytics-panel__bar-track">
                  <div
                    className="analytics-panel__bar-fill analytics-panel__bar-fill--tool"
                    style={{ width: `${(tool.count / maxToolCount) * 100}%` }}
                  />
                </div>
                <span className="analytics-panel__tool-count">{tool.count}</span>
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}

function formatDuration(seconds: number): string {
  if (seconds < 60) return `${Math.round(seconds)}s`;
  if (seconds < 3600) return `${Math.floor(seconds / 60)}m`;
  const hours = Math.floor(seconds / 3600);
  const mins = Math.floor((seconds % 3600) / 60);
  return mins > 0 ? `${hours}h ${mins}m` : `${hours}h`;
}

function formatTokens(count: number): string {
  if (count >= 1_000_000) return `${(count / 1_000_000).toFixed(1)}M`;
  if (count >= 1_000) return `${(count / 1_000).toFixed(1)}K`;
  return String(count);
}
