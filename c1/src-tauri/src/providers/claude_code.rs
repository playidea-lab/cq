//! Claude Code session provider
//!
//! Reads sessions from `~/.claude/projects/{slug}/*.jsonl`

use std::fs;
use std::path::Path;

use crate::models::{ContentBlock, SessionMeta, SessionMessage, SessionPage};
use super::{ProviderInfo, ProviderKind, SessionProvider, TokenUsage};

pub struct ClaudeCodeProvider;

/// Derive a slug from a project path: /Users/foo/bar -> -Users-foo-bar
fn path_to_slug(path: &str) -> String {
    let slug = path.replace('/', "-").replace('\\', "-").replace('_', "-");
    if slug.starts_with('-') {
        slug
    } else {
        format!("-{}", slug)
    }
}

/// Get the sessions directory for a project
fn sessions_dir(project_path: &str) -> Result<std::path::PathBuf, String> {
    let home = dirs::home_dir().ok_or("Could not find home directory")?;
    let slug = path_to_slug(project_path);
    Ok(home.join(".claude").join("projects").join(&slug))
}

impl SessionProvider for ClaudeCodeProvider {
    fn info(&self, project_path: &str) -> Result<ProviderInfo, String> {
        let dir = sessions_dir(project_path)?;
        let count = if dir.exists() {
            fs::read_dir(&dir)
                .map(|entries| {
                    entries.filter_map(|e| e.ok()).filter(|e| {
                        let p = e.path();
                        p.extension().and_then(|x| x.to_str()) == Some("jsonl")
                            && p.file_stem()
                                .and_then(|n| n.to_str())
                                .map(|n| n.len() >= 36)
                                .unwrap_or(false)
                    }).count()
                })
                .unwrap_or(0)
        } else {
            0
        };

        Ok(ProviderInfo {
            kind: ProviderKind::ClaudeCode,
            name: "Claude Code".to_string(),
            icon: "C".to_string(),
            session_count: count,
            data_path: dir.to_string_lossy().to_string(),
            is_global: false,
        })
    }

    fn list_sessions(&self, project_path: &str) -> Result<Vec<SessionMeta>, String> {
        let dir = sessions_dir(project_path)?;
        let slug = path_to_slug(project_path);

        if !dir.exists() {
            return Ok(Vec::new());
        }

        let mut sessions = Vec::new();

        for entry in fs::read_dir(&dir).map_err(|e| format!("Read dir failed: {}", e))? {
            let entry = entry.map_err(|e| format!("Entry error: {}", e))?;
            let file_path = entry.path();

            if file_path.extension().and_then(|e| e.to_str()) != Some("jsonl") {
                continue;
            }

            let file_name = file_path.file_stem()
                .and_then(|n| n.to_str())
                .unwrap_or("")
                .to_string();

            // Skip non-UUID filenames
            if file_name.len() < 36 {
                continue;
            }

            let metadata = fs::metadata(&file_path)
                .map_err(|e| format!("Metadata error: {}", e))?;

            let file_size = metadata.len();
            let modified = metadata.modified().ok()
                .and_then(|t| t.duration_since(std::time::SystemTime::UNIX_EPOCH).ok())
                .map(|d| d.as_millis() as i64);

            let (title, git_branch, session_slug) = extract_session_meta(&file_path);

            sessions.push(SessionMeta {
                id: file_name,
                slug: session_slug.unwrap_or_else(|| slug.clone()),
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

        // Validate path
        let home = dirs::home_dir().ok_or("Could not find home directory")?;
        let allowed = vec![home.join(".claude").join("projects")];
        validate_allowed_path(session_path, &allowed)?;

        let file = fs::File::open(session_path)
            .map_err(|e| format!("Failed to open session: {}", e))?;

        let reader = BufReader::new(file);
        let mut all_messages: Vec<SessionMessage> = Vec::new();
        let mut total_lines: u32 = 0;

        for line in reader.lines() {
            let line = match line {
                Ok(l) => l,
                Err(_) => continue,
            };
            total_lines += 1;

            let is_displayable = line.contains("\"type\":\"user\"")
                || line.contains("\"type\":\"assistant\"")
                || line.contains("\"type\":\"summary\"")
                || line.contains("\"type\":\"system\"")
                || line.contains("\"type\": \"user\"")
                || line.contains("\"type\": \"assistant\"")
                || line.contains("\"type\": \"summary\"")
                || line.contains("\"type\": \"system\"");

            if !is_displayable {
                continue;
            }

            if let Ok(obj) = serde_json::from_str::<serde_json::Value>(&line) {
                let msg_type = obj.get("type").and_then(|v| v.as_str()).unwrap_or("unknown");
                if let Some(msg) = parse_session_message(&obj, msg_type) {
                    all_messages.push(msg);
                }
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

    fn token_usage(&self, project_path: &str) -> Option<TokenUsage> {
        let dir = sessions_dir(project_path).ok()?;
        if !dir.exists() {
            return None;
        }

        let mut usage = TokenUsage::default();
        let entries = fs::read_dir(&dir).ok()?;

        for entry in entries.flatten() {
            let path = entry.path();
            if path.extension().and_then(|e| e.to_str()) != Some("jsonl") {
                continue;
            }
            if path.file_stem().and_then(|n| n.to_str()).map(|n| n.len() < 36).unwrap_or(true) {
                continue;
            }

            if let Some(session_usage) = extract_session_token_usage(&path) {
                usage.input_tokens += session_usage.input_tokens;
                usage.output_tokens += session_usage.output_tokens;
                usage.cache_read_tokens += session_usage.cache_read_tokens;
                usage.cache_creation_tokens += session_usage.cache_creation_tokens;
                usage.session_count += 1;
            }
        }

        if usage.session_count > 0 {
            Some(usage)
        } else {
            None
        }
    }
}

/// Extract token usage from a single session JSONL file.
/// Fast scan: only parses lines containing "input_tokens".
fn extract_session_token_usage(path: &Path) -> Option<TokenUsage> {
    use std::io::{BufRead, BufReader};

    let file = fs::File::open(path).ok()?;
    let reader = BufReader::new(file);
    let mut usage = TokenUsage { session_count: 1, ..Default::default() };
    let mut found = false;

    for line in reader.lines() {
        let line = match line {
            Ok(l) => l,
            Err(_) => continue,
        };

        // Fast filter: only parse lines with usage data
        if !line.contains("input_tokens") {
            continue;
        }

        let obj: serde_json::Value = match serde_json::from_str(&line) {
            Ok(v) => v,
            Err(_) => continue,
        };

        // Claude Code format: {"type":"assistant","message":{"usage":{...}}}
        let msg_usage = obj.get("message")
            .and_then(|m| m.get("usage"));

        if let Some(u) = msg_usage {
            found = true;
            usage.input_tokens += u.get("input_tokens").and_then(|v| v.as_u64()).unwrap_or(0);
            usage.output_tokens += u.get("output_tokens").and_then(|v| v.as_u64()).unwrap_or(0);
            usage.cache_read_tokens += u.get("cache_read_input_tokens").and_then(|v| v.as_u64()).unwrap_or(0);
            usage.cache_creation_tokens += u.get("cache_creation_input_tokens").and_then(|v| v.as_u64()).unwrap_or(0);
        }
    }

    if found { Some(usage) } else { None }
}

/// Extract metadata from a session file efficiently.
fn extract_session_meta(path: &Path) -> (Option<String>, Option<String>, Option<String>) {
    use std::io::{BufRead, BufReader, Read, Seek, SeekFrom};

    let file = match fs::File::open(path) {
        Ok(f) => f,
        Err(_) => return (None, None, None),
    };

    let mut title: Option<String> = None;
    let mut git_branch: Option<String> = None;
    let mut slug: Option<String> = None;

    let mut reader = BufReader::new(file);
    let mut line_buf = String::new();
    for _ in 0..20 {
        line_buf.clear();
        match reader.read_line(&mut line_buf) {
            Ok(0) | Err(_) => break,
            _ => {}
        }
        if let Ok(obj) = serde_json::from_str::<serde_json::Value>(&line_buf) {
            let msg_type = obj.get("type").and_then(|v| v.as_str()).unwrap_or("");
            match msg_type {
                "system" | "user" | "assistant" => {
                    if git_branch.is_none() {
                        git_branch = obj.get("gitBranch")
                            .and_then(|v| v.as_str())
                            .map(String::from);
                    }
                    if slug.is_none() {
                        slug = obj.get("slug")
                            .and_then(|v| v.as_str())
                            .map(String::from);
                    }
                }
                "summary" => {
                    title = obj.get("summary")
                        .and_then(|v| v.as_str())
                        .map(String::from);
                }
                _ => {}
            }
        }
        if git_branch.is_some() && slug.is_some() && title.is_some() {
            return (title, git_branch, slug);
        }
    }

    if title.is_none() {
        let inner = reader.into_inner();
        let file_len = inner.metadata().map(|m| m.len()).unwrap_or(0);
        if file_len > 0 {
            let mut file = inner;
            let tail_size: u64 = 65536;
            let seek_pos = if file_len > tail_size { file_len - tail_size } else { 0 };
            if file.seek(SeekFrom::Start(seek_pos)).is_ok() {
                let mut tail_buf = Vec::new();
                let _ = file.read_to_end(&mut tail_buf);
                let tail_str = String::from_utf8_lossy(&tail_buf);
                let lines = if seek_pos > 0 {
                    tail_str.splitn(2, '\n').nth(1).unwrap_or("")
                } else {
                    &tail_str
                };
                for line in lines.lines().rev() {
                    if let Ok(obj) = serde_json::from_str::<serde_json::Value>(line) {
                        if obj.get("type").and_then(|v| v.as_str()) == Some("summary") {
                            title = obj.get("summary")
                                .and_then(|v| v.as_str())
                                .map(String::from);
                            break;
                        }
                    }
                }
            }
        }
    }

    (title, git_branch, slug)
}

/// Parse a single session message JSON into SessionMessage
pub fn parse_session_message(obj: &serde_json::Value, msg_type: &str) -> Option<SessionMessage> {
    let timestamp = obj.get("timestamp").and_then(|v| v.as_str()).map(String::from);
    let uuid = obj.get("uuid").and_then(|v| v.as_str()).map(String::from);

    let content = match msg_type {
        "user" => {
            let msg = obj.get("message")?;
            if let Some(content_str) = msg.get("content").and_then(|v| v.as_str()) {
                vec![ContentBlock {
                    block_type: "text".to_string(),
                    text: Some(content_str.to_string()),
                    tool_name: None,
                    tool_input: None,
                }]
            } else if let Some(content_arr) = msg.get("content").and_then(|v| v.as_array()) {
                parse_content_blocks(content_arr)
            } else if let Some(s) = msg.as_str() {
                vec![ContentBlock {
                    block_type: "text".to_string(),
                    text: Some(s.to_string()),
                    tool_name: None,
                    tool_input: None,
                }]
            } else {
                vec![]
            }
        }
        "assistant" => {
            let msg = obj.get("message")?;
            let content_arr = msg.get("content").and_then(|v| v.as_array())?;
            parse_content_blocks(content_arr)
        }
        "summary" => {
            let summary = obj.get("summary").and_then(|v| v.as_str())?;
            vec![ContentBlock {
                block_type: "text".to_string(),
                text: Some(summary.to_string()),
                tool_name: None,
                tool_input: None,
            }]
        }
        "system" => {
            let content = obj.get("content").and_then(|v| v.as_str())?;
            vec![ContentBlock {
                block_type: "text".to_string(),
                text: Some(content.to_string()),
                tool_name: None,
                tool_input: None,
            }]
        }
        _ => vec![],
    };

    Some(SessionMessage {
        msg_type: msg_type.to_string(),
        timestamp,
        uuid,
        content,
    })
}

/// Parse content blocks array
pub fn parse_content_blocks(blocks: &[serde_json::Value]) -> Vec<ContentBlock> {
    blocks.iter().filter_map(|block| {
        let block_type = block.get("type").and_then(|v| v.as_str())?;

        match block_type {
            "text" => Some(ContentBlock {
                block_type: "text".to_string(),
                text: block.get("text").and_then(|v| v.as_str()).map(String::from),
                tool_name: None,
                tool_input: None,
            }),
            "thinking" => Some(ContentBlock {
                block_type: "thinking".to_string(),
                text: block.get("thinking").and_then(|v| v.as_str()).map(String::from),
                tool_name: None,
                tool_input: None,
            }),
            "tool_use" => Some(ContentBlock {
                block_type: "tool_use".to_string(),
                text: None,
                tool_name: block.get("name").and_then(|v| v.as_str()).map(String::from),
                tool_input: block.get("input").cloned(),
            }),
            "tool_result" => Some(ContentBlock {
                block_type: "tool_result".to_string(),
                text: block.get("content").and_then(|v| {
                    if let Some(s) = v.as_str() {
                        Some(s.to_string())
                    } else if let Some(arr) = v.as_array() {
                        Some(arr.iter()
                            .filter_map(|item| item.get("text").and_then(|t| t.as_str()))
                            .collect::<Vec<_>>()
                            .join("\n"))
                    } else {
                        None
                    }
                }),
                tool_name: block.get("tool_use_id").and_then(|v| v.as_str()).map(String::from),
                tool_input: None,
            }),
            _ => None,
        }
    }).collect()
}

/// Validate that a path falls within allowed directories
fn validate_allowed_path(file_path: &str, allowed_prefixes: &[std::path::PathBuf]) -> Result<std::path::PathBuf, String> {
    let path = Path::new(file_path);
    if !path.exists() {
        return Err(format!("File not found: {}", file_path));
    }
    let canonical = fs::canonicalize(path)
        .map_err(|e| format!("Cannot resolve path: {}", e))?;
    for prefix in allowed_prefixes {
        if let Ok(canonical_prefix) = fs::canonicalize(prefix) {
            if canonical.starts_with(&canonical_prefix) {
                return Ok(canonical);
            }
        }
    }
    Err(format!("Access denied: path outside allowed directories"))
}
