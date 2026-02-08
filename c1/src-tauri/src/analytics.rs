//! Session analytics — per-session stats and provider timeline
//!
//! Provides IPC commands for computing token usage, tool counts,
//! duration, and daily aggregation across sessions.

use std::collections::HashMap;
use std::fs;

use serde::{Deserialize, Serialize};

use crate::providers::{self, ProviderKind};

/// Per-session statistics computed from the JSONL file.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct SessionStats {
    pub total_messages: u32,
    pub user_messages: u32,
    pub assistant_messages: u32,
    pub total_input_tokens: u64,
    pub total_output_tokens: u64,
    pub cache_read_tokens: u64,
    pub estimated_cost_usd: f64,
    pub tool_calls: Vec<ToolUsageStat>,
    pub duration_seconds: f64,
    pub files_changed: u32,
}

/// A single tool's usage count.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ToolUsageStat {
    pub tool_name: String,
    pub count: u32,
}

/// Aggregated stats for one calendar day.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct DayStats {
    pub date: String, // "2026-02-08"
    pub session_count: u32,
    pub total_tokens: u64,
    pub estimated_cost: f64,
}

// ---- Cost constants (Sonnet pricing) ----
const COST_PER_INPUT_MTOK: f64 = 3.0;
const COST_PER_OUTPUT_MTOK: f64 = 15.0;

/// Compute cost in USD from token counts.
fn compute_cost(input_tokens: u64, output_tokens: u64) -> f64 {
    let input_cost = (input_tokens as f64 / 1_000_000.0) * COST_PER_INPUT_MTOK;
    let output_cost = (output_tokens as f64 / 1_000_000.0) * COST_PER_OUTPUT_MTOK;
    // Round to 4 decimal places
    ((input_cost + output_cost) * 10000.0).round() / 10000.0
}

/// Get detailed statistics for a single session.
///
/// Parses the full JSONL file to extract message counts, token usage,
/// tool call frequencies, duration, and file change count.
#[tauri::command(rename_all = "camelCase")]
pub async fn get_session_stats(
    session_path: String,
    provider: ProviderKind,
) -> Result<SessionStats, String> {
    let _ = provider; // Currently only used for routing; stats parsing is format-aware
    tokio::task::spawn_blocking(move || parse_session_stats(&session_path))
        .await
        .map_err(|e| format!("Task execution failed: {}", e))?
}

/// Get a timeline of daily session activity for a provider.
///
/// Groups sessions by file-modified date and aggregates counts, tokens, cost.
/// Returns the last `days` entries, filling gaps with zero-value entries.
#[tauri::command(rename_all = "camelCase")]
pub async fn get_provider_timeline(
    path: String,
    provider: ProviderKind,
    days: u32,
) -> Result<Vec<DayStats>, String> {
    tokio::task::spawn_blocking(move || build_provider_timeline(&path, provider, days))
        .await
        .map_err(|e| format!("Task execution failed: {}", e))?
}

// ---- Internal parsing ----

/// Parse a single JSONL session file into SessionStats.
fn parse_session_stats(session_path: &str) -> Result<SessionStats, String> {
    use std::io::{BufRead, BufReader};

    let file = fs::File::open(session_path)
        .map_err(|e| format!("Failed to open session: {}", e))?;
    let reader = BufReader::new(file);

    let mut total_messages: u32 = 0;
    let mut user_messages: u32 = 0;
    let mut assistant_messages: u32 = 0;
    let mut total_input_tokens: u64 = 0;
    let mut total_output_tokens: u64 = 0;
    let mut cache_read_tokens: u64 = 0;
    let mut tool_counts: HashMap<String, u32> = HashMap::new();
    let mut files_changed_set: std::collections::HashSet<String> = std::collections::HashSet::new();
    let mut first_timestamp: Option<String> = None;
    let mut last_timestamp: Option<String> = None;

    for line in reader.lines() {
        let line = match line {
            Ok(l) => l,
            Err(_) => continue,
        };

        // Fast pre-filter: skip lines that are not JSON objects
        if !line.starts_with('{') {
            continue;
        }

        let obj: serde_json::Value = match serde_json::from_str(&line) {
            Ok(v) => v,
            Err(_) => continue,
        };

        let msg_type = obj.get("type").and_then(|v| v.as_str()).unwrap_or("");

        // Track timestamps
        if let Some(ts) = obj.get("timestamp").and_then(|v| v.as_str()) {
            if first_timestamp.is_none() {
                first_timestamp = Some(ts.to_string());
            }
            last_timestamp = Some(ts.to_string());
        }

        match msg_type {
            "user" => {
                total_messages += 1;
                user_messages += 1;
            }
            "assistant" => {
                total_messages += 1;
                assistant_messages += 1;

                // Extract token usage from message.usage
                if let Some(usage) = obj.get("message").and_then(|m| m.get("usage")) {
                    total_input_tokens +=
                        usage.get("input_tokens").and_then(|v| v.as_u64()).unwrap_or(0);
                    total_output_tokens +=
                        usage.get("output_tokens").and_then(|v| v.as_u64()).unwrap_or(0);
                    cache_read_tokens += usage
                        .get("cache_read_input_tokens")
                        .and_then(|v| v.as_u64())
                        .unwrap_or(0);
                }

                // Count tool_use blocks in content
                if let Some(content) = obj
                    .get("message")
                    .and_then(|m| m.get("content"))
                    .and_then(|c| c.as_array())
                {
                    for block in content {
                        if block.get("type").and_then(|v| v.as_str()) == Some("tool_use") {
                            if let Some(name) = block.get("name").and_then(|v| v.as_str()) {
                                *tool_counts.entry(name.to_string()).or_insert(0) += 1;
                            }
                        }
                    }
                }
            }
            "file-history-snapshot" => {
                // Count unique file paths from snapshot.trackedFileBackups
                if let Some(backups) = obj
                    .get("snapshot")
                    .and_then(|s| s.get("trackedFileBackups"))
                    .and_then(|b| b.as_object())
                {
                    for file_path in backups.keys() {
                        files_changed_set.insert(file_path.clone());
                    }
                }
            }
            _ => {}
        }
    }

    // Compute duration from first to last timestamp
    let duration_seconds = compute_duration(&first_timestamp, &last_timestamp);

    // Build top-10 tool usage sorted by count descending
    let mut tool_vec: Vec<(String, u32)> = tool_counts.into_iter().collect();
    tool_vec.sort_by(|a, b| b.1.cmp(&a.1));
    tool_vec.truncate(10);
    let tool_calls: Vec<ToolUsageStat> = tool_vec
        .into_iter()
        .map(|(tool_name, count)| ToolUsageStat { tool_name, count })
        .collect();

    let estimated_cost_usd = compute_cost(total_input_tokens, total_output_tokens);

    Ok(SessionStats {
        total_messages,
        user_messages,
        assistant_messages,
        total_input_tokens,
        total_output_tokens,
        cache_read_tokens,
        estimated_cost_usd,
        tool_calls,
        duration_seconds,
        files_changed: files_changed_set.len() as u32,
    })
}

/// Compute duration in seconds between two ISO-8601 timestamp strings.
fn compute_duration(first: &Option<String>, last: &Option<String>) -> f64 {
    let (Some(first_str), Some(last_str)) = (first.as_ref(), last.as_ref()) else {
        return 0.0;
    };

    // Parse ISO-8601 timestamps (e.g. "2026-02-08T14:30:00.000Z")
    let first_dt = chrono::DateTime::parse_from_rfc3339(first_str)
        .or_else(|_| {
            // Try without timezone (append Z)
            chrono::DateTime::parse_from_rfc3339(&format!("{}Z", first_str))
        })
        .ok();
    let last_dt = chrono::DateTime::parse_from_rfc3339(last_str)
        .or_else(|_| chrono::DateTime::parse_from_rfc3339(&format!("{}Z", last_str)))
        .ok();

    match (first_dt, last_dt) {
        (Some(f), Some(l)) => {
            let diff = l.signed_duration_since(f);
            diff.num_seconds().max(0) as f64
        }
        _ => 0.0,
    }
}

/// Build a timeline of daily stats for a provider over the last N days.
fn build_provider_timeline(
    project_path: &str,
    provider: ProviderKind,
    days: u32,
) -> Result<Vec<DayStats>, String> {
    // Use the provider to list sessions and get their paths
    let p = providers::get_provider(provider);
    let sessions = p.list_sessions(project_path)?;

    // Group sessions by date (from file modified time)
    let mut day_map: HashMap<String, (u32, u64, u64)> = HashMap::new(); // date -> (count, input_tok, output_tok)

    for session in &sessions {
        // Get date from timestamp (millis since epoch)
        let date_str = if let Some(ts) = session.timestamp {
            let secs = ts / 1000;
            let dt = chrono::DateTime::from_timestamp(secs, 0);
            dt.map(|d| d.format("%Y-%m-%d").to_string())
        } else {
            None
        };

        let date = match date_str {
            Some(d) => d,
            None => continue,
        };

        let entry = day_map.entry(date).or_insert((0, 0, 0));
        entry.0 += 1;

        // Quick token extraction from the file
        if let Some((input, output)) = quick_token_sum(&session.path) {
            entry.1 += input;
            entry.2 += output;
        }
    }

    // Build result for last N days, filling gaps with zeros
    let today = chrono::Local::now().date_naive();
    let mut result = Vec::with_capacity(days as usize);

    for i in (0..days).rev() {
        let date = today - chrono::Duration::days(i as i64);
        let date_str = date.format("%Y-%m-%d").to_string();

        let (session_count, input_tok, output_tok) = day_map
            .get(&date_str)
            .copied()
            .unwrap_or((0, 0, 0));

        let total_tokens = input_tok + output_tok;
        let estimated_cost = compute_cost(input_tok, output_tok);

        result.push(DayStats {
            date: date_str,
            session_count,
            total_tokens,
            estimated_cost,
        });
    }

    Ok(result)
}

/// Quick extraction of total input/output tokens from a session file.
/// Only parses lines containing "input_tokens" for speed.
fn quick_token_sum(path: &str) -> Option<(u64, u64)> {
    use std::io::{BufRead, BufReader};

    let file = fs::File::open(path).ok()?;
    let reader = BufReader::new(file);
    let mut input_total: u64 = 0;
    let mut output_total: u64 = 0;
    let mut found = false;

    for line in reader.lines() {
        let line = match line {
            Ok(l) => l,
            Err(_) => continue,
        };

        if !line.contains("input_tokens") {
            continue;
        }

        let obj: serde_json::Value = match serde_json::from_str(&line) {
            Ok(v) => v,
            Err(_) => continue,
        };

        if let Some(usage) = obj.get("message").and_then(|m| m.get("usage")) {
            found = true;
            input_total += usage
                .get("input_tokens")
                .and_then(|v| v.as_u64())
                .unwrap_or(0);
            output_total += usage
                .get("output_tokens")
                .and_then(|v| v.as_u64())
                .unwrap_or(0);
        }
    }

    if found {
        Some((input_total, output_total))
    } else {
        None
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_compute_cost() {
        // 1M input tokens = $3, 1M output tokens = $15
        assert_eq!(compute_cost(1_000_000, 0), 3.0);
        assert_eq!(compute_cost(0, 1_000_000), 15.0);
        assert_eq!(compute_cost(1_000_000, 1_000_000), 18.0);
        assert_eq!(compute_cost(0, 0), 0.0);
    }

    #[test]
    fn test_compute_duration() {
        let first = Some("2026-02-08T10:00:00Z".to_string());
        let last = Some("2026-02-08T10:30:00Z".to_string());
        assert_eq!(compute_duration(&first, &last), 1800.0);

        // None timestamps
        assert_eq!(compute_duration(&None, &last), 0.0);
        assert_eq!(compute_duration(&first, &None), 0.0);
        assert_eq!(compute_duration(&None, &None), 0.0);
    }

    #[test]
    fn test_compute_duration_same_time() {
        let ts = Some("2026-02-08T12:00:00Z".to_string());
        assert_eq!(compute_duration(&ts, &ts), 0.0);
    }
}
