//! Codex CLI session provider
//!
//! Reads sessions from `~/.codex/sessions/YYYY/MM/DD/*.jsonl`
//!
//! Codex JSONL format:
//! - `session_meta`: session metadata (id, cwd, model, cli_version, etc.)
//! - `response_item`: messages (user/developer/assistant), function_calls, reasoning
//! - `event_msg`: user_message events, token counts, agent reasoning
//! - `turn_context`: per-turn metadata (model, cwd, effort)

use std::fs;
use std::path::{Path, PathBuf};

use crate::models::{ContentBlock, SessionMeta, SessionMessage, SessionPage};
use super::{ProviderInfo, ProviderKind, SessionProvider};

pub struct CodexCliProvider;

/// Get the codex sessions root directory
fn codex_sessions_dir() -> Result<PathBuf, String> {
    let home = dirs::home_dir().ok_or("Could not find home directory")?;
    Ok(home.join(".codex").join("sessions"))
}

/// Recursively find all .jsonl files under a directory
fn find_jsonl_files(dir: &Path) -> Vec<PathBuf> {
    let mut files = Vec::new();
    if !dir.exists() {
        return files;
    }
    if let Ok(entries) = fs::read_dir(dir) {
        for entry in entries.flatten() {
            let path = entry.path();
            if path.is_dir() {
                files.extend(find_jsonl_files(&path));
            } else if path.extension().and_then(|e| e.to_str()) == Some("jsonl") {
                files.push(path);
            }
        }
    }
    files
}

/// Extract metadata from a Codex session file.
/// Reads first few lines to find session_meta and first user message.
fn extract_codex_meta(path: &Path) -> Option<(String, Option<String>, Option<String>, Option<String>)> {
    use std::io::{BufRead, BufReader};

    let file = fs::File::open(path).ok()?;
    let reader = BufReader::new(file);

    let mut session_id: Option<String> = None;
    let mut cwd: Option<String> = None;
    let mut model: Option<String> = None;
    let mut first_user_msg: Option<String> = None;

    for (i, line) in reader.lines().enumerate() {
        if i > 30 { break; } // Don't read too many lines
        let line = match line {
            Ok(l) => l,
            Err(_) => continue,
        };

        let obj: serde_json::Value = match serde_json::from_str(&line) {
            Ok(v) => v,
            Err(_) => continue,
        };

        let msg_type = obj.get("type").and_then(|v| v.as_str()).unwrap_or("");

        match msg_type {
            "session_meta" => {
                if let Some(payload) = obj.get("payload") {
                    session_id = payload.get("id").and_then(|v| v.as_str()).map(String::from);
                    cwd = payload.get("cwd").and_then(|v| v.as_str()).map(String::from);
                }
            }
            "event_msg" => {
                if let Some(payload) = obj.get("payload") {
                    if payload.get("type").and_then(|v| v.as_str()) == Some("user_message") {
                        if first_user_msg.is_none() {
                            first_user_msg = payload.get("message")
                                .and_then(|v| v.as_str())
                                .map(|s| {
                                    let trimmed = s.trim();
                                    if trimmed.len() > 120 {
                                        let end = trimmed.char_indices()
                                            .take_while(|(i, _)| *i <= 120)
                                            .last()
                                            .map(|(i, _)| i)
                                            .unwrap_or(0);
                                        format!("{}...", &trimmed[..end])
                                    } else {
                                        trimmed.to_string()
                                    }
                                });
                        }
                    }
                }
            }
            "turn_context" => {
                if model.is_none() {
                    if let Some(payload) = obj.get("payload") {
                        model = payload.get("model").and_then(|v| v.as_str()).map(String::from);
                    }
                }
            }
            _ => {}
        }

        if session_id.is_some() && first_user_msg.is_some() && model.is_some() {
            break;
        }
    }

    // Extract session ID from filename as fallback
    let file_id = session_id.unwrap_or_else(|| {
        path.file_stem()
            .and_then(|n| n.to_str())
            .unwrap_or("unknown")
            .to_string()
    });

    Some((file_id, first_user_msg, cwd, model))
}

impl SessionProvider for CodexCliProvider {
    fn info(&self, _project_path: &str) -> Result<ProviderInfo, String> {
        let dir = codex_sessions_dir()?;
        let count = if dir.exists() {
            find_jsonl_files(&dir).len()
        } else {
            0
        };

        // Only report if actually installed (has sessions dir or config)
        let home = dirs::home_dir().ok_or("Could not find home directory")?;
        let codex_dir = home.join(".codex");
        if !codex_dir.exists() {
            return Err("Codex CLI not installed".to_string());
        }

        Ok(ProviderInfo {
            kind: ProviderKind::CodexCli,
            name: "Codex CLI".to_string(),
            icon: "X".to_string(),
            session_count: count,
            data_path: dir.to_string_lossy().to_string(),
        })
    }

    fn list_sessions(&self, _project_path: &str) -> Result<Vec<SessionMeta>, String> {
        let dir = codex_sessions_dir()?;
        if !dir.exists() {
            return Ok(Vec::new());
        }

        let files = find_jsonl_files(&dir);
        let mut sessions = Vec::new();

        for file_path in &files {
            let metadata = match fs::metadata(file_path) {
                Ok(m) => m,
                Err(_) => continue,
            };

            let file_size = metadata.len();
            let modified = metadata.modified().ok()
                .and_then(|t| t.duration_since(std::time::SystemTime::UNIX_EPOCH).ok())
                .map(|d| d.as_millis() as i64);

            let (id, title, cwd, model) = match extract_codex_meta(file_path) {
                Some(m) => m,
                None => continue,
            };

            // Use cwd as git_branch stand-in (shows project context)
            let git_branch = model;

            sessions.push(SessionMeta {
                id,
                slug: cwd.unwrap_or_default(),
                title,
                path: file_path.to_string_lossy().to_string(),
                line_count: 0,
                file_size,
                timestamp: modified,
                git_branch,
            });
        }

        sessions.sort_by(|a, b| b.timestamp.cmp(&a.timestamp));

        Ok(sessions)
    }

    fn get_messages(
        &self,
        session_path: &str,
        offset: u32,
        limit: u32,
    ) -> Result<SessionPage, String> {
        use std::io::{BufRead, BufReader};

        let path = Path::new(session_path);
        if !path.exists() {
            return Err(format!("File not found: {}", session_path));
        }

        // Validate path is under ~/.codex/
        let home = dirs::home_dir().ok_or("Could not find home directory")?;
        let codex_dir = home.join(".codex");
        let canonical = fs::canonicalize(path)
            .map_err(|e| format!("Cannot resolve path: {}", e))?;
        let canonical_codex = fs::canonicalize(&codex_dir)
            .map_err(|e| format!("Cannot resolve codex dir: {}", e))?;
        if !canonical.starts_with(&canonical_codex) {
            return Err("Access denied: path outside ~/.codex/".to_string());
        }

        let file = fs::File::open(path)
            .map_err(|e| format!("Failed to open session: {}", e))?;

        let reader = BufReader::new(file);
        let mut messages = Vec::new();
        let mut total_lines: u32 = 0;
        let mut displayable_count: u32 = 0;

        for line in reader.lines() {
            let line = match line {
                Ok(l) => l,
                Err(_) => continue,
            };
            total_lines += 1;

            let obj: serde_json::Value = match serde_json::from_str(&line) {
                Ok(v) => v,
                Err(_) => continue,
            };

            let line_type = obj.get("type").and_then(|v| v.as_str()).unwrap_or("");

            match line_type {
                "event_msg" => {
                    if let Some(msg) = parse_codex_event_msg(&obj) {
                        displayable_count += 1;
                        if displayable_count > offset && messages.len() < limit as usize {
                            messages.push(msg);
                        }
                    }
                }
                "response_item" => {
                    if let Some(msg) = parse_codex_response_item(&obj) {
                        displayable_count += 1;
                        if displayable_count > offset && messages.len() < limit as usize {
                            messages.push(msg);
                        }
                    }
                }
                _ => {}
            }
        }

        let has_more = displayable_count > offset + messages.len() as u32;

        Ok(SessionPage {
            messages,
            total_lines,
            has_more,
        })
    }
}

/// Parse a Codex `event_msg` into a SessionMessage (user messages, reasoning)
fn parse_codex_event_msg(obj: &serde_json::Value) -> Option<SessionMessage> {
    let payload = obj.get("payload")?;
    let timestamp = obj.get("timestamp").and_then(|v| v.as_str()).map(String::from);
    let event_type = payload.get("type").and_then(|v| v.as_str())?;

    match event_type {
        "user_message" => {
            let text = payload.get("message").and_then(|v| v.as_str())?;
            if text.trim().is_empty() {
                return None;
            }
            Some(SessionMessage {
                msg_type: "user".to_string(),
                timestamp,
                uuid: None,
                content: vec![ContentBlock {
                    block_type: "text".to_string(),
                    text: Some(text.to_string()),
                    tool_name: None,
                    tool_input: None,
                }],
            })
        }
        "agent_reasoning" => {
            let text = payload.get("text").and_then(|v| v.as_str())?;
            if text.trim().is_empty() {
                return None;
            }
            Some(SessionMessage {
                msg_type: "assistant".to_string(),
                timestamp,
                uuid: None,
                content: vec![ContentBlock {
                    block_type: "thinking".to_string(),
                    text: Some(text.to_string()),
                    tool_name: None,
                    tool_input: None,
                }],
            })
        }
        _ => None,
    }
}

/// Parse a Codex `response_item` into a SessionMessage
fn parse_codex_response_item(obj: &serde_json::Value) -> Option<SessionMessage> {
    let payload = obj.get("payload")?;
    let timestamp = obj.get("timestamp").and_then(|v| v.as_str()).map(String::from);
    let item_type = payload.get("type").and_then(|v| v.as_str())?;

    match item_type {
        "message" => {
            let role = payload.get("role").and_then(|v| v.as_str()).unwrap_or("unknown");

            // Skip developer/system prompt messages
            if role == "developer" || role == "system" {
                return None;
            }

            let content_arr = payload.get("content").and_then(|v| v.as_array())?;
            let blocks = parse_codex_content_blocks(content_arr);

            if blocks.is_empty() {
                return None;
            }

            let msg_type = match role {
                "user" => "user",
                "assistant" => "assistant",
                _ => return None,
            };

            Some(SessionMessage {
                msg_type: msg_type.to_string(),
                timestamp,
                uuid: None,
                content: blocks,
            })
        }
        "function_call" => {
            let name = payload.get("name").and_then(|v| v.as_str()).unwrap_or("unknown");
            let arguments = payload.get("arguments").and_then(|v| v.as_str());

            // Parse arguments JSON string into Value
            let tool_input = arguments.and_then(|args| {
                serde_json::from_str::<serde_json::Value>(args).ok()
            });

            Some(SessionMessage {
                msg_type: "assistant".to_string(),
                timestamp,
                uuid: payload.get("call_id").and_then(|v| v.as_str()).map(String::from),
                content: vec![ContentBlock {
                    block_type: "tool_use".to_string(),
                    text: None,
                    tool_name: Some(name.to_string()),
                    tool_input,
                }],
            })
        }
        "function_call_output" => {
            let output = payload.get("output").and_then(|v| v.as_str())?;

            Some(SessionMessage {
                msg_type: "assistant".to_string(),
                timestamp,
                uuid: None,
                content: vec![ContentBlock {
                    block_type: "tool_result".to_string(),
                    text: Some(output.to_string()),
                    tool_name: payload.get("call_id").and_then(|v| v.as_str()).map(String::from),
                    tool_input: None,
                }],
            })
        }
        "reasoning" => {
            // Reasoning summaries
            let summaries = payload.get("summary").and_then(|v| v.as_array())?;
            let text: Vec<String> = summaries.iter()
                .filter_map(|s| s.get("text").and_then(|v| v.as_str()))
                .map(String::from)
                .collect();

            if text.is_empty() {
                return None;
            }

            Some(SessionMessage {
                msg_type: "assistant".to_string(),
                timestamp,
                uuid: None,
                content: vec![ContentBlock {
                    block_type: "thinking".to_string(),
                    text: Some(text.join("\n")),
                    tool_name: None,
                    tool_input: None,
                }],
            })
        }
        _ => None,
    }
}

/// Parse Codex content blocks (input_text, output_text)
fn parse_codex_content_blocks(blocks: &[serde_json::Value]) -> Vec<ContentBlock> {
    blocks.iter().filter_map(|block| {
        let block_type = block.get("type").and_then(|v| v.as_str())?;

        match block_type {
            "input_text" | "output_text" => {
                let text = block.get("text").and_then(|v| v.as_str())?;
                // Skip very long system/developer prompts
                if text.len() > 5000 && block_type == "input_text" {
                    return None;
                }
                Some(ContentBlock {
                    block_type: "text".to_string(),
                    text: Some(text.to_string()),
                    tool_name: None,
                    tool_input: None,
                })
            }
            _ => None,
        }
    }).collect()
}
