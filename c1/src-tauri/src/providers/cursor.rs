//! Cursor session provider
//!
//! Reads sessions from Cursor's state.vscdb SQLite database.
//!
//! Database: `~/Library/Application Support/Cursor/User/globalStorage/state.vscdb`
//! Table: `cursorDiskKV`
//! Keys:
//! - `composerData:{uuid}` → session metadata JSON
//! - `bubbleId:{composer}:{bubble}` → message JSON (type 1=user, 2=assistant)

use std::path::PathBuf;

use rusqlite::{Connection, OpenFlags};

use crate::models::{ContentBlock, SessionMeta, SessionMessage, SessionPage};
use super::{ProviderInfo, ProviderKind, SessionProvider};

pub struct CursorProvider;

/// Get the Cursor state database path
fn cursor_db_path() -> Result<PathBuf, String> {
    let home = dirs::home_dir().ok_or("Could not find home directory")?;
    #[cfg(target_os = "macos")]
    let path = home
        .join("Library")
        .join("Application Support")
        .join("Cursor")
        .join("User")
        .join("globalStorage")
        .join("state.vscdb");

    #[cfg(target_os = "linux")]
    let path = home
        .join(".config")
        .join("Cursor")
        .join("User")
        .join("globalStorage")
        .join("state.vscdb");

    #[cfg(target_os = "windows")]
    let path = home
        .join("AppData")
        .join("Roaming")
        .join("Cursor")
        .join("User")
        .join("globalStorage")
        .join("state.vscdb");

    Ok(path)
}

/// Open the Cursor database in READONLY mode
fn open_cursor_db() -> Result<Connection, String> {
    let path = cursor_db_path()?;
    if !path.exists() {
        return Err("Cursor database not found".to_string());
    }
    Connection::open_with_flags(&path, OpenFlags::SQLITE_OPEN_READ_ONLY)
        .map_err(|e| format!("Failed to open Cursor DB: {}", e))
}

impl SessionProvider for CursorProvider {
    fn info(&self, _project_path: &str) -> Result<ProviderInfo, String> {
        let db_path = cursor_db_path()?;
        if !db_path.exists() {
            return Err("Cursor not installed".to_string());
        }

        let count = match open_cursor_db() {
            Ok(conn) => {
                conn.query_row(
                    "SELECT COUNT(*) FROM cursorDiskKV WHERE key LIKE 'composerData:%'",
                    [],
                    |row| row.get::<_, usize>(0),
                ).unwrap_or(0)
            }
            Err(_) => 0,
        };

        Ok(ProviderInfo {
            kind: ProviderKind::Cursor,
            name: "Cursor".to_string(),
            icon: "U".to_string(),
            session_count: count,
            data_path: db_path.to_string_lossy().to_string(),
            is_global: true,
        })
    }

    fn list_sessions(&self, _project_path: &str) -> Result<Vec<SessionMeta>, String> {
        let conn = open_cursor_db()?;

        let mut stmt = conn
            .prepare(
                "SELECT key, value FROM cursorDiskKV WHERE key LIKE 'composerData:%'"
            )
            .map_err(|e| format!("Failed to prepare query: {}", e))?;

        let rows = stmt
            .query_map([], |row| {
                Ok((
                    row.get::<_, String>(0)?,
                    row.get::<_, String>(1)?,
                ))
            })
            .map_err(|e| format!("Failed to query composers: {}", e))?;

        let mut sessions = Vec::new();

        for row in rows {
            let (key, value) = match row {
                Ok(r) => r,
                Err(_) => continue,
            };

            let composer_id = key.strip_prefix("composerData:").unwrap_or(&key).to_string();

            let data: serde_json::Value = match serde_json::from_str(&value) {
                Ok(v) => v,
                Err(_) => continue,
            };

            let name = data.get("name")
                .and_then(|v| v.as_str())
                .map(String::from);

            let subtitle = data.get("subtitle")
                .and_then(|v| v.as_str())
                .map(String::from);

            let title = name.or(subtitle);

            let created_at = data.get("createdAt")
                .and_then(|v| v.as_i64());

            let last_updated = data.get("lastUpdatedAt")
                .and_then(|v| v.as_i64());

            let timestamp = last_updated.or(created_at);

            let status = data.get("status")
                .and_then(|v| v.as_str())
                .unwrap_or("unknown");

            let mode = data.get("unifiedMode")
                .and_then(|v| v.as_str())
                .or_else(|| data.get("forceMode").and_then(|v| v.as_str()));

            let model = data.get("modelConfig")
                .and_then(|v| v.get("modelName"))
                .and_then(|v| v.as_str());

            // Compute bubble count from headers
            let bubble_count = data.get("fullConversationHeadersOnly")
                .and_then(|v| v.as_array())
                .map(|a| a.len() as u32)
                .unwrap_or(0);

            let git_branch = model.map(String::from)
                .or_else(|| mode.map(String::from));

            sessions.push(SessionMeta {
                id: composer_id.clone(),
                slug: status.to_string(),
                title,
                path: composer_id, // Use composer UUID as path (get_messages looks up by ID)
                line_count: bubble_count,
                file_size: value.len() as u64,
                timestamp,
                git_branch,
            });
        }

        sessions.sort_by(|a, b| b.timestamp.cmp(&a.timestamp));

        Ok(sessions)
    }

    fn get_messages(
        &self,
        session_id: &str,
        offset: u32,
        limit: u32,
    ) -> Result<SessionPage, String> {
        let conn = open_cursor_db()?;

        // 1. Get composer data for bubble ordering
        let composer_key = if session_id.contains(':') {
            session_id.to_string()
        } else {
            format!("composerData:{}", session_id)
        };

        // Extract composer_id from key or raw id
        let composer_id = composer_key
            .strip_prefix("composerData:")
            .unwrap_or(session_id);

        let composer_json: String = conn
            .query_row(
                "SELECT value FROM cursorDiskKV WHERE key = ?",
                [&composer_key],
                |row| row.get(0),
            )
            .map_err(|e| format!("Composer not found: {}", e))?;

        let composer: serde_json::Value = serde_json::from_str(&composer_json)
            .map_err(|e| format!("Invalid composer JSON: {}", e))?;

        // Get ordered bubble IDs from headers
        let headers = composer.get("fullConversationHeadersOnly")
            .and_then(|v| v.as_array())
            .cloned()
            .unwrap_or_default();

        let total_lines = headers.len() as u32;
        let mut all_messages: Vec<SessionMessage> = Vec::new();

        for header in &headers {
            let bubble_id = match header.get("bubbleId").and_then(|v| v.as_str()) {
                Some(id) => id,
                None => continue,
            };

            let bubble_type = header.get("type").and_then(|v| v.as_i64()).unwrap_or(0);

            // Fetch bubble data
            let bubble_key = format!("bubbleId:{}:{}", composer_id, bubble_id);
            let bubble_json: String = match conn.query_row(
                "SELECT value FROM cursorDiskKV WHERE key = ?",
                [&bubble_key],
                |row| row.get(0),
            ) {
                Ok(v) => v,
                Err(_) => continue,
            };

            let bubble: serde_json::Value = match serde_json::from_str(&bubble_json) {
                Ok(v) => v,
                Err(_) => continue,
            };

            if let Some(msg) = parse_cursor_bubble(&bubble, bubble_type) {
                all_messages.push(msg);
            }
        }

        // Paginate from end: offset=0 → newest PAGE_SIZE messages
        let total = all_messages.len() as u32;
        let has_more = total > offset + limit;
        let start = total.saturating_sub(offset + limit) as usize;
        let end = total.saturating_sub(offset) as usize;
        let mut messages = all_messages[start..end].to_vec();
        messages.reverse(); // newest first

        Ok(SessionPage {
            messages,
            total_lines,
            has_more,
        })
    }
}

/// Parse a Cursor bubble into a SessionMessage
fn parse_cursor_bubble(bubble: &serde_json::Value, bubble_type: i64) -> Option<SessionMessage> {
    let timestamp = bubble.get("createdAt")
        .and_then(|v| v.as_str())
        .map(String::from);

    let msg_type = match bubble_type {
        1 => "user",
        2 => "assistant",
        _ => return None,
    };

    let mut content = Vec::new();

    // Text content
    let text = bubble.get("text").and_then(|v| v.as_str()).unwrap_or("");
    if !text.is_empty() {
        content.push(ContentBlock {
            block_type: "text".to_string(),
            text: Some(text.to_string()),
            tool_name: None,
            tool_input: None,
        });
    }

    // Thinking blocks
    if let Some(thinking_blocks) = bubble.get("allThinkingBlocks").and_then(|v| v.as_array()) {
        for tb in thinking_blocks {
            if let Some(thinking_text) = tb.get("thinking").and_then(|v| v.as_str()) {
                if !thinking_text.is_empty() {
                    content.push(ContentBlock {
                        block_type: "thinking".to_string(),
                        text: Some(thinking_text.to_string()),
                        tool_name: None,
                        tool_input: None,
                    });
                }
            }
        }
    }

    // Tool calls via toolFormerData
    if let Some(tool_data) = bubble.get("toolFormerData").and_then(|v| v.as_object()) {
        let tool_name = tool_data.get("name")
            .and_then(|v| v.as_str())
            .unwrap_or("unknown");

        let tool_input = tool_data.get("rawArgs")
            .and_then(|v| v.as_str())
            .and_then(|s| serde_json::from_str::<serde_json::Value>(s).ok());

        content.push(ContentBlock {
            block_type: "tool_use".to_string(),
            text: None,
            tool_name: Some(tool_name.to_string()),
            tool_input,
        });

        // Tool result
        if let Some(result) = tool_data.get("result").and_then(|v| v.as_str()) {
            let truncated = if result.len() > 500 {
                let end = result.char_indices()
                    .take_while(|(i, _)| *i <= 500)
                    .last()
                    .map(|(i, _)| i)
                    .unwrap_or(0);
                format!("{}... ({} bytes)", &result[..end], result.len())
            } else {
                result.to_string()
            };
            content.push(ContentBlock {
                block_type: "tool_result".to_string(),
                text: Some(truncated),
                tool_name: Some(tool_name.to_string()),
                tool_input: None,
            });
        }
    }

    // Tool results array
    if let Some(tool_results) = bubble.get("toolResults").and_then(|v| v.as_array()) {
        for tr in tool_results {
            if let Some(tr_obj) = tr.as_object() {
                let name = tr_obj.get("toolName")
                    .and_then(|v| v.as_str())
                    .unwrap_or("tool");

                let input = tr_obj.get("args")
                    .and_then(|v| v.as_str())
                    .and_then(|s| serde_json::from_str::<serde_json::Value>(s).ok());

                content.push(ContentBlock {
                    block_type: "tool_use".to_string(),
                    text: None,
                    tool_name: Some(name.to_string()),
                    tool_input: input,
                });
            }
        }
    }

    if content.is_empty() {
        return None;
    }

    Some(SessionMessage {
        msg_type: msg_type.to_string(),
        timestamp,
        uuid: bubble.get("bubbleId").and_then(|v| v.as_str()).map(String::from),
        content,
    })
}
