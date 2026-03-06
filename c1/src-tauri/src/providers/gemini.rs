//! Gemini CLI session provider
//!
//! Sessions: ~/.gemini/tmp/{name_or_sha256}/chats/session-*.json
//! ~/.gemini/projects.json maps project paths to short names;
//! unregistered projects fall back to SHA256(path).

use crate::models::{ContentBlock, SessionMeta, SessionMessage, SessionPage};
use crate::providers::{ProviderInfo, ProviderKind, SessionProvider, TokenUsage};
use serde::Deserialize;
use sha2::{Digest, Sha256};
use std::fs;

pub struct GeminiProvider;

fn project_key(project_path: &str) -> String {
    // Check ~/.gemini/projects.json for registered short name
    if let Some(home) = dirs::home_dir() {
        let projects_file = home.join(".gemini").join("projects.json");
        if let Ok(text) = fs::read_to_string(&projects_file) {
            if let Ok(val) = serde_json::from_str::<serde_json::Value>(&text) {
                if let Some(name) = val["projects"][project_path].as_str() {
                    return name.to_string();
                }
            }
        }
    }
    // Fallback: SHA256 of path
    let mut hasher = Sha256::new();
    hasher.update(project_path.as_bytes());
    format!("{:x}", hasher.finalize())
}

fn chats_dir(project_path: &str) -> Option<std::path::PathBuf> {
    let home = dirs::home_dir()?;
    let key = project_key(project_path);
    Some(home.join(".gemini").join("tmp").join(key).join("chats"))
}

// Raw Gemini session JSON structures
#[derive(Deserialize)]
struct GeminiSession {
    #[serde(rename = "sessionId")]
    session_id: String,
    #[serde(rename = "startTime")]
    start_time: Option<String>,
    #[serde(rename = "lastUpdated")]
    last_updated: Option<String>,
    messages: Vec<GeminiMessage>,
}

#[derive(Deserialize)]
struct GeminiMessage {
    id: Option<String>,
    timestamp: Option<String>,
    #[serde(rename = "type")]
    msg_type: String,
    // content can be a plain string OR [{text: "..."}] array
    content: Option<serde_json::Value>,
    #[serde(rename = "toolCalls")]
    tool_calls: Option<Vec<GeminiToolCall>>,
}

#[derive(Deserialize)]
struct GeminiToolCall {
    name: Option<String>,
    args: Option<serde_json::Value>,
    #[serde(rename = "resultDisplay")]
    result_display: Option<String>,
}

fn parse_gemini_message(msg: &GeminiMessage) -> SessionMessage {
    let role = match msg.msg_type.as_str() {
        "user" => "user",
        _ => "assistant",
    };

    let mut blocks: Vec<ContentBlock> = Vec::new();

    // Text content: plain string or [{text:"..."}] array
    let text_content = match &msg.content {
        Some(serde_json::Value::String(s)) if !s.is_empty() => Some(s.clone()),
        Some(serde_json::Value::Array(arr)) => {
            let joined: String = arr.iter()
                .filter_map(|v| v.get("text").and_then(|t| t.as_str()))
                .collect::<Vec<_>>()
                .join("\n");
            if joined.is_empty() { None } else { Some(joined) }
        }
        _ => None,
    };
    if let Some(text) = text_content {
        blocks.push(ContentBlock {
            block_type: "text".to_string(),
            text: Some(text),
            tool_name: None,
            tool_input: None,
        });
    }

    // Tool calls (assistant messages)
    if let Some(calls) = &msg.tool_calls {
        for call in calls {
            blocks.push(ContentBlock {
                block_type: "tool_use".to_string(),
                text: call.result_display.clone(),
                tool_name: call.name.clone(),
                tool_input: call.args.clone(),
            });
        }
    }

    SessionMessage {
        msg_type: role.to_string(),
        timestamp: msg.timestamp.clone(),
        uuid: msg.id.clone(),
        content: blocks,
    }
}

impl SessionProvider for GeminiProvider {
    fn info(&self, project_path: &str) -> Result<ProviderInfo, String> {
        let dir = chats_dir(project_path).ok_or("No home directory")?;
        if !dir.exists() {
            return Err("Gemini chats directory not found".to_string());
        }

        let count = fs::read_dir(&dir)
            .map_err(|e| e.to_string())?
            .filter_map(|e| e.ok())
            .filter(|e| {
                e.file_name()
                    .to_string_lossy()
                    .starts_with("session-")
            })
            .count();

        Ok(ProviderInfo {
            kind: ProviderKind::GeminiCli,
            name: "Gemini CLI".to_string(),
            icon: "G".to_string(),
            session_count: count,
            data_path: dir.to_string_lossy().to_string(),
            is_global: false,
        })
    }

    fn list_sessions(&self, project_path: &str) -> Result<Vec<SessionMeta>, String> {
        let dir = chats_dir(project_path).ok_or("No home directory")?;
        if !dir.exists() {
            return Ok(vec![]);
        }

        let mut sessions: Vec<SessionMeta> = fs::read_dir(&dir)
            .map_err(|e| e.to_string())?
            .filter_map(|e| e.ok())
            .filter(|e| {
                let name = e.file_name();
                let s = name.to_string_lossy();
                s.starts_with("session-") && s.ends_with(".json")
            })
            .filter_map(|entry| {
                let path = entry.path();
                let text = fs::read_to_string(&path).ok()?;
                let session: GeminiSession = serde_json::from_str(&text).ok()?;

                let timestamp = session
                    .last_updated
                    .as_deref()
                    .or(session.start_time.as_deref())
                    .and_then(|s| chrono::DateTime::parse_from_rfc3339(s).ok())
                    .map(|dt| dt.timestamp());

                // Use first user message as title
                let title = session.messages.iter()
                    .find(|m| m.msg_type == "user")
                    .and_then(|m| match &m.content {
                        Some(serde_json::Value::String(s)) if !s.is_empty() => Some(s.clone()),
                        Some(serde_json::Value::Array(arr)) => {
                            let t: String = arr.iter()
                                .filter_map(|v| v.get("text").and_then(|t| t.as_str()))
                                .collect::<Vec<_>>().join(" ");
                            if t.is_empty() { None } else { Some(t) }
                        }
                        _ => None,
                    })
                    .map(|s| {
                        let t: String = s.chars().take(60).collect();
                        if s.chars().count() > 60 { format!("{}...", t) } else { t }
                    });

                let meta = fs::metadata(&path).ok()?;

                Some(SessionMeta {
                    id: session.session_id,
                    slug: entry.file_name().to_string_lossy().to_string(),
                    title,
                    path: path.to_string_lossy().to_string(),
                    line_count: session.messages.len() as u32,
                    file_size: meta.len(),
                    timestamp,
                    git_branch: None,
                })
            })
            .collect();

        sessions.sort_by(|a, b| b.timestamp.cmp(&a.timestamp));
        Ok(sessions)
    }

    fn get_messages(&self, session_path: &str, offset: u32, limit: u32) -> Result<SessionPage, String> {
        let text = fs::read_to_string(session_path)
            .map_err(|e| format!("Failed to open session: {}", e))?;

        let session: GeminiSession = serde_json::from_str(&text)
            .map_err(|e| format!("Invalid Gemini session JSON: {}", e))?;

        let all_messages: Vec<SessionMessage> = session
            .messages
            .iter()
            .filter(|m| !m.msg_type.is_empty())
            .map(parse_gemini_message)
            .filter(|m| !m.content.is_empty())
            .collect();

        // Paginate from end: offset=0 → newest PAGE_SIZE messages
        let total = all_messages.len() as u32;
        let has_more = total > offset + limit;
        let start = total.saturating_sub(offset + limit) as usize;
        let end = total.saturating_sub(offset) as usize;
        let mut messages = all_messages[start..end].to_vec();
        messages.reverse(); // newest first

        Ok(SessionPage {
            messages,
            total_lines: total,
            has_more,
        })
    }

    fn token_usage(&self, _project_path: &str) -> Option<TokenUsage> {
        None
    }
}
