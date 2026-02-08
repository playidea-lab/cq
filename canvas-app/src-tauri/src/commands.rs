//! Tauri IPC commands

use std::collections::HashMap;
use std::fs;
use std::path::Path;

use rusqlite::Connection;

use crate::models::{
    CanvasData, ConfigFileContent, ConfigFileEntry, ContentBlock, FileChange, ProjectState,
    ScanResult, SessionMeta, SessionMessage, SessionPage, TaskDetail, TaskItem, TaskProgress,
    WorkerInfo,
};
use crate::providers::{self, ProviderInfo, ProviderKind};
use crate::scanner::{get_project_id, scan_project};

/// Canvas save file name
const CANVAS_FILE: &str = ".c4/canvas.json";

/// Scan a project directory and return canvas data
#[tauri::command(rename_all = "camelCase")]
pub async fn scan_project_cmd(path: String) -> ScanResult {
    let project_path = Path::new(&path).to_path_buf();

    // Quick sync checks before spawning blocking task
    if !project_path.exists() {
        return ScanResult::err(format!("Path does not exist: {}", path));
    }

    if !project_path.is_dir() {
        return ScanResult::err(format!("Path is not a directory: {}", path));
    }

    // Wrap heavy I/O operations in spawn_blocking to prevent UI freeze
    match tokio::task::spawn_blocking(move || scan_project(&project_path)).await {
        Ok(Ok(data)) => ScanResult::ok(data),
        Ok(Err(e)) => ScanResult::err(format!("Scan failed: {}", e)),
        Err(e) => ScanResult::err(format!("Task execution failed: {}", e)),
    }
}

/// Save canvas state to project
#[tauri::command]
pub fn save_canvas(path: String, data: CanvasData) -> Result<(), String> {
    let canvas_path = Path::new(&path).join(CANVAS_FILE);

    // Ensure .c4 directory exists
    if let Some(parent) = canvas_path.parent() {
        fs::create_dir_all(parent).map_err(|e| format!("Failed to create directory: {}", e))?;
    }

    let json = serde_json::to_string_pretty(&data)
        .map_err(|e| format!("Failed to serialize canvas: {}", e))?;

    fs::write(&canvas_path, json).map_err(|e| format!("Failed to write canvas file: {}", e))?;

    Ok(())
}

/// Load canvas state from project
#[tauri::command]
pub fn load_canvas(path: String) -> Option<CanvasData> {
    let canvas_path = Path::new(&path).join(CANVAS_FILE);

    if !canvas_path.exists() {
        return None;
    }

    let content = fs::read_to_string(&canvas_path).ok()?;
    serde_json::from_str(&content).ok()
}

// --- Dashboard API commands ---

/// Open the C4 SQLite database
fn open_c4_db(project_path: &Path) -> Result<Connection, String> {
    let db_path = project_path.join(".c4").join("c4.db");
    if !db_path.exists() {
        return Err("C4 database not found".to_string());
    }
    Connection::open(&db_path).map_err(|e| format!("Failed to open DB: {}", e))
}

/// Map DB task type to frontend enum value
fn map_task_type(raw: &str) -> String {
    match raw {
        "impl" => "IMPLEMENTATION".to_string(),
        "review" => "REVIEW".to_string(),
        "checkpoint" => "CHECKPOINT".to_string(),
        other => other.to_uppercase(),
    }
}

/// Extract a string array from a JSON value
fn json_string_array(val: Option<&serde_json::Value>) -> Vec<String> {
    val.and_then(|v| v.as_array())
        .map(|arr| {
            arr.iter()
                .filter_map(|item| item.as_str().map(String::from))
                .collect()
        })
        .unwrap_or_default()
}

/// Get project state including status, workers, and progress
#[tauri::command(rename_all = "camelCase")]
pub async fn get_project_state(path: String) -> Result<ProjectState, String> {
    let project_path = path.clone();
    tokio::task::spawn_blocking(move || {
        let project_path = Path::new(&project_path);
        let conn = open_c4_db(project_path)?;
        let project_id = get_project_id(project_path)
            .map_err(|e| format!("Failed to get project_id: {}", e))?;

        // 1. Read state_json
        let state_json: String = conn
            .query_row(
                "SELECT state_json FROM c4_state WHERE project_id = ?",
                [&project_id],
                |row| row.get(0),
            )
            .map_err(|e| format!("Failed to read state: {}", e))?;

        let state: serde_json::Value =
            serde_json::from_str(&state_json).map_err(|e| format!("Invalid state JSON: {}", e))?;

        let status = state
            .get("status")
            .and_then(|v| v.as_str())
            .unwrap_or("UNKNOWN")
            .to_string();

        // 2. Extract workers from c4_tasks (assigned_to with in_progress status)
        let mut worker_stmt = conn
            .prepare(
                "SELECT DISTINCT assigned_to, task_id FROM c4_tasks \
                 WHERE project_id = ? AND status = 'in_progress' AND assigned_to IS NOT NULL",
            )
            .map_err(|e| format!("Failed to prepare worker query: {}", e))?;

        let mut worker_map: HashMap<String, Vec<String>> = HashMap::new();
        let rows = worker_stmt
            .query_map([&project_id], |row| {
                Ok((
                    row.get::<_, String>(0)?,
                    row.get::<_, String>(1)?,
                ))
            })
            .map_err(|e| format!("Failed to query workers: {}", e))?;

        for row in rows {
            if let Ok((worker_id, task_id)) = row {
                worker_map.entry(worker_id).or_default().push(task_id);
            }
        }

        let workers: Vec<WorkerInfo> = worker_map
            .into_iter()
            .map(|(id, tasks)| WorkerInfo {
                id,
                status: "busy".to_string(),
                current_task: tasks.into_iter().next(),
                last_seen: None,
            })
            .collect();

        // 3. Count tasks by status
        let mut count_stmt = conn
            .prepare(
                "SELECT status, COUNT(*) FROM c4_tasks WHERE project_id = ? GROUP BY status",
            )
            .map_err(|e| format!("Failed to prepare count query: {}", e))?;

        let mut done: u32 = 0;
        let mut in_progress: u32 = 0;
        let mut pending: u32 = 0;

        let count_rows = count_stmt
            .query_map([&project_id], |row| {
                Ok((row.get::<_, String>(0)?, row.get::<_, u32>(1)?))
            })
            .map_err(|e| format!("Failed to count tasks: {}", e))?;

        for row in count_rows {
            if let Ok((s, c)) = row {
                match s.as_str() {
                    "done" => done = c,
                    "in_progress" => in_progress = c,
                    "pending" => pending = c,
                    _ => {}
                }
            }
        }

        // blocked from repair_queue in state_json
        let blocked = state
            .get("repair_queue")
            .and_then(|v| v.as_array())
            .map(|a| a.len() as u32)
            .unwrap_or(0);

        Ok(ProjectState {
            status,
            project_id,
            workers,
            progress: TaskProgress {
                total: done + in_progress + pending,
                done,
                in_progress,
                pending,
                blocked,
            },
        })
    })
    .await
    .map_err(|e| format!("Task execution failed: {}", e))?
}

/// Get all tasks as a list
#[tauri::command(rename_all = "camelCase")]
pub async fn get_tasks(path: String) -> Result<Vec<TaskItem>, String> {
    let project_path = path.clone();
    tokio::task::spawn_blocking(move || {
        let project_path = Path::new(&project_path);
        let conn = open_c4_db(project_path)?;
        let project_id = get_project_id(project_path)
            .map_err(|e| format!("Failed to get project_id: {}", e))?;

        let mut stmt = conn
            .prepare(
                "SELECT task_id, task_json, status, assigned_to FROM c4_tasks WHERE project_id = ?",
            )
            .map_err(|e| format!("Failed to prepare query: {}", e))?;

        let rows = stmt
            .query_map([&project_id], |row| {
                Ok((
                    row.get::<_, String>(0)?,
                    row.get::<_, String>(1)?,
                    row.get::<_, String>(2)?,
                    row.get::<_, Option<String>>(3)?,
                ))
            })
            .map_err(|e| format!("Failed to query tasks: {}", e))?;

        let mut tasks = Vec::new();
        for row in rows {
            let (task_id, task_json, status, assigned_to) =
                row.map_err(|e| format!("Row error: {}", e))?;

            let data: serde_json::Value =
                serde_json::from_str(&task_json).unwrap_or_default();

            tasks.push(TaskItem {
                id: task_id,
                title: data
                    .get("title")
                    .and_then(|v| v.as_str())
                    .unwrap_or("")
                    .to_string(),
                status,
                task_type: map_task_type(
                    data.get("type").and_then(|v| v.as_str()).unwrap_or("impl"),
                ),
                dependencies: json_string_array(data.get("dependencies")),
                assigned_to,
                domain: data
                    .get("domain")
                    .and_then(|v| v.as_str())
                    .map(String::from),
                priority: data
                    .get("priority")
                    .and_then(|v| v.as_i64())
                    .unwrap_or(0) as i32,
            });
        }

        Ok(tasks)
    })
    .await
    .map_err(|e| format!("Task execution failed: {}", e))?
}

/// Get detailed info for a single task
#[tauri::command(rename_all = "camelCase")]
pub async fn get_task_detail(path: String, task_id: String) -> Result<Option<TaskDetail>, String> {
    let project_path = path.clone();
    tokio::task::spawn_blocking(move || {
        let project_path = Path::new(&project_path);
        let conn = open_c4_db(project_path)?;
        let project_id = get_project_id(project_path)
            .map_err(|e| format!("Failed to get project_id: {}", e))?;

        let result = conn.query_row(
            "SELECT task_json, status, assigned_to FROM c4_tasks WHERE project_id = ? AND task_id = ?",
            [&project_id, &task_id],
            |row| {
                Ok((
                    row.get::<_, String>(0)?,
                    row.get::<_, String>(1)?,
                    row.get::<_, Option<String>>(2)?,
                ))
            },
        );

        match result {
            Ok((task_json, status, assigned_to)) => {
                let data: serde_json::Value =
                    serde_json::from_str(&task_json).unwrap_or_default();

                Ok(Some(TaskDetail {
                    id: task_id,
                    title: data.get("title").and_then(|v| v.as_str()).unwrap_or("").to_string(),
                    status,
                    task_type: map_task_type(
                        data.get("type").and_then(|v| v.as_str()).unwrap_or("impl"),
                    ),
                    dependencies: json_string_array(data.get("dependencies")),
                    assigned_to,
                    domain: data.get("domain").and_then(|v| v.as_str()).map(String::from),
                    priority: data.get("priority").and_then(|v| v.as_i64()).unwrap_or(0) as i32,
                    dod: data.get("dod").and_then(|v| v.as_str()).unwrap_or("").to_string(),
                    scope: data.get("scope").and_then(|v| v.as_str()).map(String::from),
                    branch: data.get("branch").and_then(|v| v.as_str()).map(String::from),
                    commit_sha: data.get("commit_sha").and_then(|v| v.as_str()).map(String::from),
                    version: data.get("version").and_then(|v| v.as_i64()).unwrap_or(0) as i32,
                    parent_id: data.get("parent_id").and_then(|v| v.as_str()).map(String::from),
                    review_decision: data.get("review_decision").and_then(|v| v.as_str()).map(String::from),
                    validations: json_string_array(data.get("validations")),
                }))
            }
            Err(rusqlite::Error::QueryReturnedNoRows) => Ok(None),
            Err(e) => Err(format!("Failed to query task: {}", e)),
        }
    })
    .await
    .map_err(|e| format!("Task execution failed: {}", e))?
}

// --- Session API commands ---

/// Derive a slug from a project path: /Users/foo/bar -> -Users-foo-bar
fn path_to_slug(path: &str) -> String {
    let slug = path.replace('/', "-").replace('\\', "-");
    if slug.starts_with('-') {
        slug
    } else {
        format!("-{}", slug)
    }
}

/// List all sessions for a project
#[tauri::command(rename_all = "camelCase")]
pub async fn list_sessions(path: String) -> Result<Vec<SessionMeta>, String> {
    let project_path = path.clone();
    tokio::task::spawn_blocking(move || {
        let home = dirs::home_dir().ok_or("Could not find home directory")?;
        let slug = path_to_slug(&project_path);
        let sessions_dir = home.join(".claude").join("projects").join(&slug);

        if !sessions_dir.exists() {
            return Ok(Vec::new());
        }

        let mut sessions = Vec::new();

        for entry in fs::read_dir(&sessions_dir).map_err(|e| format!("Read dir failed: {}", e))? {
            let entry = entry.map_err(|e| format!("Entry error: {}", e))?;
            let file_path = entry.path();

            if file_path.extension().and_then(|e| e.to_str()) != Some("jsonl") {
                continue;
            }

            let file_name = file_path.file_stem()
                .and_then(|n| n.to_str())
                .unwrap_or("")
                .to_string();

            // Skip non-UUID filenames (like memory files)
            if file_name.len() < 36 {
                continue;
            }

            let metadata = fs::metadata(&file_path)
                .map_err(|e| format!("Metadata error: {}", e))?;

            let file_size = metadata.len();
            let modified = metadata.modified().ok()
                .and_then(|t| t.duration_since(std::time::SystemTime::UNIX_EPOCH).ok())
                .map(|d| d.as_millis() as i64);

            // Read first/last lines to extract metadata (does NOT read entire file)
            let (title, git_branch, session_slug) =
                extract_session_meta(&file_path);

            sessions.push(SessionMeta {
                id: file_name,
                slug: session_slug.unwrap_or_else(|| slug.clone()),
                title,
                path: file_path.to_string_lossy().to_string(),
                line_count: 0,  // not computed for performance; use file_size
                file_size,
                timestamp: modified,
                git_branch,
            });
        }

        // Sort by timestamp descending (newest first)
        sessions.sort_by(|a, b| b.timestamp.cmp(&a.timestamp));

        Ok(sessions)
    })
    .await
    .map_err(|e| format!("Task execution failed: {}", e))?
}

/// Extract metadata from a session file efficiently.
/// Reads first 20 lines for slug/branch, then last 64KB for summary title.
/// No longer counts total lines (use file_size instead).
fn extract_session_meta(path: &Path) -> (Option<String>, Option<String>, Option<String>) {
    use std::io::{BufRead, BufReader, Read, Seek, SeekFrom};

    let file = match fs::File::open(path) {
        Ok(f) => f,
        Err(_) => return (None, None, None),
    };

    let mut title: Option<String> = None;
    let mut git_branch: Option<String> = None;
    let mut slug: Option<String> = None;

    // Phase 1: Read first 20 lines for slug and branch
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

    // Phase 2: Read last 64KB for summary title (summaries are typically near EOF)
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
                // Skip partial first line if we seeked mid-file
                let lines = if seek_pos > 0 {
                    tail_str.splitn(2, '\n').nth(1).unwrap_or("")
                } else {
                    &tail_str
                };
                // Scan lines in reverse order for the last summary
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

/// Get paginated session messages
#[tauri::command(rename_all = "camelCase")]
pub async fn get_session_messages(
    session_path: String,
    offset: u32,
    limit: u32,
) -> Result<SessionPage, String> {
    tokio::task::spawn_blocking(move || {
        use std::io::{BufRead, BufReader};

        // Validate session path is within ~/.claude/projects/
        let home = dirs::home_dir().ok_or("Could not find home directory")?;
        let allowed = vec![home.join(".claude").join("projects")];
        validate_allowed_path(&session_path, &allowed)?;

        let file = fs::File::open(&session_path)
            .map_err(|e| format!("Failed to open session: {}", e))?;

        let reader = BufReader::new(file);
        let mut messages = Vec::new();
        let mut total_lines: u32 = 0;
        let mut displayable_count: u32 = 0;
        let page_filled = |msgs: &Vec<SessionMessage>| msgs.len() >= limit as usize;

        for line in reader.lines() {
            let line = match line {
                Ok(l) => l,
                Err(_) => continue,
            };
            total_lines += 1;

            // Lightweight type check: avoid full JSON parse when possible
            // Check if line contains a displayable type via string search first
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

            displayable_count += 1;

            // Before offset: count only, skip parsing
            if displayable_count <= offset {
                continue;
            }

            // After page filled: count only for has_more detection
            if page_filled(&messages) {
                continue;
            }

            // Within page: full parse
            if let Ok(obj) = serde_json::from_str::<serde_json::Value>(&line) {
                let msg_type = obj.get("type").and_then(|v| v.as_str()).unwrap_or("unknown");
                if let Some(msg) = parse_session_message(&obj, msg_type) {
                    messages.push(msg);
                }
            }
        }

        let has_more = displayable_count > offset + messages.len() as u32;

        Ok(SessionPage {
            messages,
            total_lines,
            has_more,
        })
    })
    .await
    .map_err(|e| format!("Task execution failed: {}", e))?
}

/// Parse a single session message JSON into SessionMessage
fn parse_session_message(obj: &serde_json::Value, msg_type: &str) -> Option<SessionMessage> {
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
fn parse_content_blocks(blocks: &[serde_json::Value]) -> Vec<ContentBlock> {
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

/// Get file changes from a session
#[tauri::command(rename_all = "camelCase")]
pub async fn get_session_file_changes(session_path: String) -> Result<Vec<FileChange>, String> {
    tokio::task::spawn_blocking(move || {
        use std::io::{BufRead, BufReader};

        // Validate session path is within ~/.claude/projects/
        let home = dirs::home_dir().ok_or("Could not find home directory")?;
        let allowed = vec![home.join(".claude").join("projects")];
        validate_allowed_path(&session_path, &allowed)?;

        let file = fs::File::open(&session_path)
            .map_err(|e| format!("Failed to open session: {}", e))?;

        let reader = BufReader::new(file);
        let mut changes = Vec::new();

        for line in reader.lines() {
            let line = match line {
                Ok(l) => l,
                Err(_) => continue,
            };

            if let Ok(obj) = serde_json::from_str::<serde_json::Value>(&line) {
                if obj.get("type").and_then(|v| v.as_str()) != Some("file-history-snapshot") {
                    continue;
                }

                let snapshot = match obj.get("snapshot") {
                    Some(s) => s,
                    None => continue,
                };

                let timestamp = snapshot.get("timestamp")
                    .and_then(|v| v.as_str())
                    .map(String::from);

                if let Some(backups) = snapshot.get("trackedFileBackups").and_then(|v| v.as_object()) {
                    for (file_path, info) in backups {
                        changes.push(FileChange {
                            path: file_path.clone(),
                            backup_file: info.get("backupFile")
                                .and_then(|v| v.as_str())
                                .map(String::from),
                            version: info.get("version")
                                .and_then(|v| v.as_i64())
                                .map(|v| v as i32),
                            timestamp: timestamp.clone(),
                        });
                    }
                }
            }
        }

        Ok(changes)
    })
    .await
    .map_err(|e| format!("Task execution failed: {}", e))?
}

// --- Provider API commands ---

/// List all detected LLM tool providers
#[tauri::command(rename_all = "camelCase")]
pub async fn list_providers(path: String) -> Result<Vec<ProviderInfo>, String> {
    let project_path = path;
    tokio::task::spawn_blocking(move || {
        Ok(providers::detect_providers(&project_path))
    })
    .await
    .map_err(|e| format!("Task execution failed: {}", e))?
}

/// List sessions for a specific provider
#[tauri::command(rename_all = "camelCase")]
pub async fn list_sessions_for_provider(
    path: String,
    provider: ProviderKind,
) -> Result<Vec<SessionMeta>, String> {
    let project_path = path;
    tokio::task::spawn_blocking(move || {
        let p = providers::get_provider(provider);
        p.list_sessions(&project_path)
    })
    .await
    .map_err(|e| format!("Task execution failed: {}", e))?
}

/// Get paginated session messages for a specific provider
#[tauri::command(rename_all = "camelCase")]
pub async fn get_provider_session_messages(
    session_path: String,
    provider: ProviderKind,
    offset: u32,
    limit: u32,
) -> Result<SessionPage, String> {
    tokio::task::spawn_blocking(move || {
        let p = providers::get_provider(provider);
        p.get_messages(&session_path, offset, limit)
    })
    .await
    .map_err(|e| format!("Task execution failed: {}", e))?
}

// --- Config API commands ---

/// Maximum file size to read (1MB)
const MAX_CONFIG_FILE_SIZE: u64 = 1_048_576;

/// List config files for the project explorer
#[tauri::command(rename_all = "camelCase")]
pub async fn list_config_files(path: String) -> Result<Vec<ConfigFileEntry>, String> {
    let project_path = path.clone();
    tokio::task::spawn_blocking(move || {
        let project_root = Path::new(&project_path);
        let home = dirs::home_dir().ok_or("Could not find home directory")?;
        let slug = path_to_slug(&project_path);

        let mut entries = Vec::new();

        // 1. Global: ~/.claude/CLAUDE.md, ~/.claude/rules/*.md
        collect_file(&home.join(".claude").join("CLAUDE.md"), "global", &mut entries);
        collect_glob(&home.join(".claude").join("rules"), "md", "global", &mut entries);

        // 2. Project: AGENTS.md, CLAUDE.md, .claude/rules/*.md (excluding persona-*)
        collect_file(&project_root.join("AGENTS.md"), "project", &mut entries);

        let claude_md = project_root.join("CLAUDE.md");
        let agents_md = project_root.join("AGENTS.md");
        // Only add CLAUDE.md if it's not a symlink to AGENTS.md
        if claude_md.exists() {
            let same = fs::canonicalize(&agents_md).ok()
                .and_then(|a| fs::canonicalize(&claude_md).ok().map(|c| a == c))
                .unwrap_or(false);
            if !same {
                collect_file(&claude_md, "project", &mut entries);
            }
        }

        let project_rules = project_root.join(".claude").join("rules");
        if project_rules.exists() {
            for entry in fs::read_dir(&project_rules).into_iter().flatten().flatten() {
                let p = entry.path();
                if p.is_file() && p.extension().and_then(|e| e.to_str()) == Some("md") {
                    let name = p.file_name().and_then(|n| n.to_str()).unwrap_or("");
                    let category = if name.starts_with("persona-") { "persona" } else { "project" };
                    collect_file(&p, category, &mut entries);
                }
            }
        }

        // 3. C4: .c4/config.yaml
        collect_file(&project_root.join(".c4").join("config.yaml"), "c4", &mut entries);

        // 4. Memory: ~/.claude/projects/{slug}/memory/*.md
        collect_glob(
            &home.join(".claude").join("projects").join(&slug).join("memory"),
            "md", "memory", &mut entries,
        );

        entries.sort_by(|a, b| a.category.cmp(&b.category).then(a.name.cmp(&b.name)));

        Ok(entries)
    })
    .await
    .map_err(|e| format!("Task execution failed: {}", e))?
}

/// Collect a single file into entries if it exists
fn collect_file(path: &Path, category: &str, entries: &mut Vec<ConfigFileEntry>) {
    if !path.exists() || !path.is_file() {
        return;
    }
    let metadata = fs::metadata(path).ok();
    let size = metadata.as_ref().map(|m| m.len()).unwrap_or(0);
    let modified = metadata.and_then(|m| m.modified().ok())
        .and_then(|t| t.duration_since(std::time::SystemTime::UNIX_EPOCH).ok())
        .map(|d| d.as_millis() as i64);

    entries.push(ConfigFileEntry {
        path: path.to_string_lossy().to_string(),
        name: path.file_name().and_then(|n| n.to_str()).unwrap_or("").to_string(),
        category: category.to_string(),
        size,
        modified,
    });
}

/// Collect all files with given extension from a directory
fn collect_glob(dir: &Path, ext: &str, category: &str, entries: &mut Vec<ConfigFileEntry>) {
    if !dir.exists() {
        return;
    }
    for entry in fs::read_dir(dir).into_iter().flatten().flatten() {
        let p = entry.path();
        if p.is_file() && p.extension().and_then(|e| e.to_str()) == Some(ext) {
            collect_file(&p, category, entries);
        }
    }
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

/// Read a config file content
#[tauri::command(rename_all = "camelCase")]
pub async fn read_config_file(project_path: String, file_path: String) -> Result<ConfigFileContent, String> {
    tokio::task::spawn_blocking(move || {
        let home = dirs::home_dir().ok_or("Could not find home directory")?;
        let allowed = vec![
            Path::new(&project_path).to_path_buf(),
            home.join(".claude"),
        ];
        validate_allowed_path(&file_path, &allowed)?;

        let path = Path::new(&file_path);
        if !path.exists() {
            return Err(format!("File not found: {}", file_path));
        }

        let metadata = fs::metadata(path)
            .map_err(|e| format!("Metadata error: {}", e))?;

        let truncated = metadata.len() > MAX_CONFIG_FILE_SIZE;

        let content = if truncated {
            use std::io::Read;
            let mut file = fs::File::open(path)
                .map_err(|e| format!("Open error: {}", e))?;
            let mut buf = vec![0u8; MAX_CONFIG_FILE_SIZE as usize];
            let bytes_read = file.read(&mut buf)
                .map_err(|e| format!("Read error: {}", e))?;
            buf.truncate(bytes_read);
            String::from_utf8_lossy(&buf).to_string()
        } else {
            fs::read_to_string(path)
                .map_err(|e| format!("Read error: {}", e))?
        };

        Ok(ConfigFileContent {
            path: file_path,
            content,
            truncated,
        })
    })
    .await
    .map_err(|e| format!("Task execution failed: {}", e))?
}
