//! Project scanner - extracts nodes and edges from a C4 project

use anyhow::{Context, Result};
use chrono::{TimeZone, Utc};
use regex::Regex;
use rusqlite::Connection;
use std::collections::{HashMap, HashSet};
use std::fs;
use std::path::{Path, PathBuf};
use std::time::SystemTime;
use walkdir::WalkDir;

use crate::cloud::{build_client, read_auth_token, read_supabase_config, retry_request};
use crate::layout::{apply_time_layout, resolve_overlaps};
use crate::messaging::Channel;
use crate::models::{CanvasData, CanvasEdge, CanvasNode, NodeType, Position, RelationType};

/// Patterns to scan for different node types
const SCAN_PATTERNS: &[(&str, &[&str])] = &[
    (".claude", &["**/*.md", "**/*.yaml", "**/*.json"]),
    (".c4", &["**/*.yaml", "**/*.json", "state.json", "tasks.db"]),
    ("docs", &["**/*.md"]),
    (".env", &[]),
    (".mcp.json", &[]),
];

/// Files/directories to ignore
const IGNORE_PATTERNS: &[&str] = &[
    "node_modules",
    ".git",
    ".venv",
    "__pycache__",
    "target",
    "dist",
    ".pytest_cache",
    ".mypy_cache",
    ".ruff_cache",
];

/// Maximum depth for directory scanning
const MAX_DEPTH: usize = 5;

/// Scan a project and extract canvas data
pub fn scan_project(project_path: &Path) -> Result<CanvasData> {
    let mut nodes = Vec::new();
    let mut edges = Vec::new();
    let mut node_ids: HashSet<String> = HashSet::new();

    // Scan different source directories/files
    for (pattern, _sub_patterns) in SCAN_PATTERNS {
        let target_path = project_path.join(pattern);
        if target_path.exists() {
            if target_path.is_file() {
                if let Some(node) = scan_file(&target_path, project_path)? {
                    if node_ids.insert(node.id.clone()) {
                        nodes.push(node);
                    }
                }
            } else if target_path.is_dir() {
                scan_directory(&target_path, project_path, &mut nodes, &mut node_ids)?;
            }
        }
    }

    // Scan for Claude session files
    scan_sessions(project_path, &mut nodes, &mut node_ids)?;

    // Scan C4 tasks from SQLite database
    scan_c4_tasks(project_path, &mut nodes, &mut edges, &mut node_ids)?;

    // Extract relationships between nodes
    extract_relationships(&nodes, &mut edges)?;

    // Apply time-based layout
    apply_time_layout(&mut nodes);
    resolve_overlaps(&mut nodes);

    Ok(CanvasData {
        nodes,
        edges,
        viewport: Default::default(),
    })
}

/// Scan a directory recursively
fn scan_directory(
    dir_path: &Path,
    project_root: &Path,
    nodes: &mut Vec<CanvasNode>,
    node_ids: &mut HashSet<String>,
) -> Result<()> {
    for entry in WalkDir::new(dir_path)
        .max_depth(MAX_DEPTH)
        .into_iter()
        .filter_entry(|e| !should_ignore(e.path()))
    {
        let entry = entry?;
        if entry.file_type().is_file() {
            if let Some(node) = scan_file(entry.path(), project_root)? {
                if node_ids.insert(node.id.clone()) {
                    nodes.push(node);
                }
            }
        }
    }
    Ok(())
}

/// Check if a path should be ignored
fn should_ignore(path: &Path) -> bool {
    path.components().any(|c| {
        let name = c.as_os_str().to_string_lossy();
        IGNORE_PATTERNS.iter().any(|p| name == *p)
    })
}

/// Scan a single file and create a node
fn scan_file(file_path: &Path, project_root: &Path) -> Result<Option<CanvasNode>> {
    let extension = file_path.extension().and_then(|e| e.to_str()).unwrap_or("");

    // Only process supported file types
    if !["md", "yaml", "yml", "json", "toml", "jsonl", "db"].contains(&extension) {
        // Check for dotfiles without extension
        let file_name = file_path.file_name().and_then(|n| n.to_str()).unwrap_or("");
        if !file_name.starts_with('.') || !file_name.contains("env") && !file_name.contains("mcp") {
            return Ok(None);
        }
    }

    let relative_path = file_path
        .strip_prefix(project_root)
        .unwrap_or(file_path)
        .to_string_lossy()
        .to_string();

    let file_name = file_path
        .file_name()
        .and_then(|n| n.to_str())
        .unwrap_or("unknown")
        .to_string();

    let node_type = NodeType::from_path(&relative_path);

    // Get file metadata
    let metadata = fs::metadata(file_path)?;
    let timestamp = metadata
        .modified()
        .ok()
        .and_then(|t| t.duration_since(SystemTime::UNIX_EPOCH).ok())
        .map(|d| d.as_millis() as i64);

    // Extract additional metadata based on file type
    let mut node_metadata = HashMap::new();
    extract_file_metadata(file_path, extension, &mut node_metadata)?;

    let id = generate_node_id(&relative_path);

    Ok(Some(CanvasNode {
        id,
        node_type,
        label: file_name,
        path: Some(relative_path),
        metadata: node_metadata,
        position: Position { x: 0.0, y: 0.0 }, // Will be set by layout
        timestamp,
    }))
}

/// Extract metadata from file contents
fn extract_file_metadata(
    file_path: &Path,
    extension: &str,
    metadata: &mut HashMap<String, serde_json::Value>,
) -> Result<()> {
    match extension {
        "md" => {
            // Extract title from markdown
            if let Ok(content) = fs::read_to_string(file_path) {
                if let Some(title) = extract_markdown_title(&content) {
                    metadata.insert("title".to_string(), serde_json::json!(title));
                }
                // Count lines
                metadata.insert("lines".to_string(), serde_json::json!(content.lines().count()));
            }
        }
        "yaml" | "yml" => {
            // Parse YAML and extract top-level keys
            if let Ok(content) = fs::read_to_string(file_path) {
                if let Ok(yaml) = serde_yaml::from_str::<serde_json::Value>(&content) {
                    if let Some(obj) = yaml.as_object() {
                        let keys: Vec<_> = obj.keys().cloned().collect();
                        metadata.insert("keys".to_string(), serde_json::json!(keys));
                    }
                }
            }
        }
        "json" => {
            // Parse JSON and extract top-level keys
            if let Ok(content) = fs::read_to_string(file_path) {
                if let Ok(json) = serde_json::from_str::<serde_json::Value>(&content) {
                    if let Some(obj) = json.as_object() {
                        let keys: Vec<_> = obj.keys().cloned().collect();
                        metadata.insert("keys".to_string(), serde_json::json!(keys));
                    }
                }
            }
        }
        _ => {}
    }
    Ok(())
}

/// Extract title from markdown content
fn extract_markdown_title(content: &str) -> Option<String> {
    for line in content.lines() {
        let trimmed = line.trim();
        if trimmed.starts_with("# ") {
            return Some(trimmed[2..].trim().to_string());
        }
    }
    None
}

/// Scan for Claude session files
fn scan_sessions(
    project_root: &Path,
    nodes: &mut Vec<CanvasNode>,
    node_ids: &mut HashSet<String>,
) -> Result<()> {
    // Look for session files in ~/.claude/projects/
    let home = dirs::home_dir().context("Could not find home directory")?;
    let sessions_base = home.join(".claude").join("projects");

    if !sessions_base.exists() {
        return Ok(());
    }

    // Try to find sessions related to this project
    let project_name = project_root
        .file_name()
        .and_then(|n| n.to_str())
        .unwrap_or("");

    for entry in fs::read_dir(&sessions_base)? {
        let entry = entry?;
        let dir_name = entry.file_name().to_string_lossy().to_string();

        // Match project-related session directories
        if dir_name.contains(project_name) || dir_name.contains(&project_root.to_string_lossy().replace('/', "-").replace('\\', "-")) {
            // Scan session files
            for session_entry in fs::read_dir(entry.path())? {
                let session_entry = session_entry?;
                let session_path = session_entry.path();

                if session_path.extension().and_then(|e| e.to_str()) == Some("jsonl") {
                    if let Some(node) = create_session_node(&session_path, project_root)? {
                        if node_ids.insert(node.id.clone()) {
                            nodes.push(node);
                        }
                    }
                }
            }
        }
    }

    Ok(())
}

/// Create a session node from a session file
fn create_session_node(session_path: &Path, _project_root: &Path) -> Result<Option<CanvasNode>> {
    let file_name = session_path
        .file_stem()
        .and_then(|n| n.to_str())
        .unwrap_or("session")
        .to_string();

    let metadata = fs::metadata(session_path)?;
    let timestamp = metadata
        .modified()
        .ok()
        .and_then(|t| t.duration_since(SystemTime::UNIX_EPOCH).ok())
        .map(|d| d.as_millis() as i64);

    // Try to extract last message from session
    let mut node_metadata = HashMap::new();
    if let Ok(content) = fs::read_to_string(session_path) {
        let lines: Vec<_> = content.lines().collect();
        node_metadata.insert("messages".to_string(), serde_json::json!(lines.len()));
    }

    let id = generate_node_id(&format!("session:{}", file_name));

    Ok(Some(CanvasNode {
        id,
        node_type: NodeType::Session,
        label: format!("Session {}", &file_name[..8.min(file_name.len())]),
        path: Some(session_path.to_string_lossy().to_string()),
        metadata: node_metadata,
        position: Position { x: 0.0, y: 0.0 },
        timestamp,
    }))
}

/// Generate a unique node ID from a path
fn generate_node_id(path: &str) -> String {
    use std::collections::hash_map::DefaultHasher;
    use std::hash::{Hash, Hasher};

    let mut hasher = DefaultHasher::new();
    path.hash(&mut hasher);
    format!("node_{:x}", hasher.finish())
}

/// Extract relationships between nodes
fn extract_relationships(nodes: &[CanvasNode], edges: &mut Vec<CanvasEdge>) -> Result<()> {
    let node_map: HashMap<&str, &CanvasNode> = nodes
        .iter()
        .filter_map(|n| n.path.as_ref().map(|p| (p.as_str(), n)))
        .collect();

    let reference_pattern = Regex::new(r#"(?:import|from|require|include|source)\s+['"](\.?[^'"]+)['"]"#)?;
    let markdown_link_pattern = Regex::new(r#"\[([^\]]+)\]\(([^)]+)\)"#)?;

    for node in nodes {
        if let Some(path) = &node.path {
            let full_path = if path.starts_with('/') {
                PathBuf::from(path)
            } else {
                // This is a relative path, we'd need the project root
                continue;
            };

            if let Ok(content) = fs::read_to_string(&full_path) {
                // Check for import/reference patterns
                for cap in reference_pattern.captures_iter(&content) {
                    if let Some(referenced) = cap.get(1) {
                        let referenced_path = referenced.as_str();
                        if let Some(target_node) = find_matching_node(&node_map, referenced_path) {
                            let edge_id = format!("edge_{}_{}", node.id, target_node.id);
                            edges.push(CanvasEdge {
                                id: edge_id,
                                source: node.id.clone(),
                                target: target_node.id.clone(),
                                relation: RelationType::References,
                            });
                        }
                    }
                }

                // Check for markdown links
                for cap in markdown_link_pattern.captures_iter(&content) {
                    if let Some(link_target) = cap.get(2) {
                        let link_path = link_target.as_str();
                        if !link_path.starts_with("http") {
                            if let Some(target_node) = find_matching_node(&node_map, link_path) {
                                let edge_id = format!("edge_{}_{}", node.id, target_node.id);
                                edges.push(CanvasEdge {
                                    id: edge_id,
                                    source: node.id.clone(),
                                    target: target_node.id.clone(),
                                    relation: RelationType::Mentions,
                                });
                            }
                        }
                    }
                }
            }
        }
    }

    // Add C4 task dependencies
    add_c4_task_edges(nodes, edges);

    Ok(())
}

/// Find a node matching a referenced path
fn find_matching_node<'a>(
    node_map: &'a HashMap<&str, &'a CanvasNode>,
    reference: &str,
) -> Option<&'a CanvasNode> {
    // Direct match
    if let Some(node) = node_map.get(reference) {
        return Some(*node);
    }

    // Try with common extensions
    for ext in &["", ".md", ".yaml", ".json"] {
        let with_ext = format!("{}{}", reference, ext);
        if let Some(node) = node_map.get(with_ext.as_str()) {
            return Some(*node);
        }
    }

    // Try partial match (filename only)
    let ref_filename = Path::new(reference)
        .file_name()
        .and_then(|n| n.to_str())
        .unwrap_or(reference);

    for (path, node) in node_map.iter() {
        if path.ends_with(ref_filename) {
            return Some(*node);
        }
    }

    None
}

/// Add edges for C4 task dependencies (from metadata)
fn add_c4_task_edges(nodes: &[CanvasNode], edges: &mut Vec<CanvasEdge>) {
    // Find task nodes and extract dependency information from metadata
    let task_nodes: Vec<_> = nodes
        .iter()
        .filter(|n| n.node_type == NodeType::Task || n.path.as_ref().map(|p| p.contains("tasks")).unwrap_or(false))
        .collect();

    for node in &task_nodes {
        if let Some(deps) = node.metadata.get("dependencies") {
            if let Some(dep_array) = deps.as_array() {
                for dep in dep_array {
                    if let Some(dep_id) = dep.as_str() {
                        // Find matching task node
                        if let Some(dep_node) = task_nodes.iter().find(|n| {
                            n.metadata
                                .get("id")
                                .and_then(|v| v.as_str())
                                .map(|id| id == dep_id)
                                .unwrap_or(false)
                        }) {
                            let edge_id = format!("edge_{}_{}", node.id, dep_node.id);
                            edges.push(CanvasEdge {
                                id: edge_id,
                                source: node.id.clone(),
                                target: dep_node.id.clone(),
                                relation: RelationType::Depends,
                            });
                        }
                    }
                }
            }
        }
    }
}

/// Get project_id from .c4/config.yaml or use directory name as fallback
pub fn get_project_id(project_root: &Path) -> Result<String> {
    let config_path = project_root.join(".c4").join("config.yaml");

    // Try to read project_id from config.yaml
    if config_path.exists() {
        if let Ok(content) = fs::read_to_string(&config_path) {
            if let Ok(yaml) = serde_yaml::from_str::<serde_json::Value>(&content) {
                if let Some(project_id) = yaml.get("project_id").and_then(|v| v.as_str()) {
                    return Ok(project_id.to_string());
                }
            }
        }
    }

    // Fallback: use directory name
    let dir_name = project_root
        .file_name()
        .and_then(|n| n.to_str())
        .unwrap_or("default")
        .to_string();

    Ok(dir_name)
}

/// Scan C4 tasks from the SQLite database
fn scan_c4_tasks(
    project_root: &Path,
    nodes: &mut Vec<CanvasNode>,
    edges: &mut Vec<CanvasEdge>,
    node_ids: &mut HashSet<String>,
) -> Result<()> {
    let db_path = project_root.join(".c4").join("c4.db");

    if !db_path.exists() {
        return Ok(());
    }

    // Get project_id from config.yaml or use directory name as fallback
    let project_id = get_project_id(project_root)?;

    let conn = Connection::open(&db_path)
        .context("Failed to open C4 database")?;

    // Query tasks from c4_tasks table using parameterized query
    let mut stmt = conn.prepare(
        "SELECT task_id, task_json, status, updated_at FROM c4_tasks WHERE project_id = ?"
    )?;

    let task_iter = stmt.query_map([&project_id], |row| {
        Ok(C4TaskRow {
            task_id: row.get(0)?,
            task_json: row.get(1)?,
            status: row.get(2)?,
            updated_at: row.get::<_, Option<String>>(3)?,
        })
    })?;

    // Build a map of task_id -> node_id for dependency resolution
    let mut task_node_map: HashMap<String, String> = HashMap::new();

    for task_result in task_iter {
        let task_row = task_result?;

        // Parse task JSON
        let task_data: serde_json::Value = serde_json::from_str(&task_row.task_json)
            .unwrap_or_default();

        let title = task_data.get("title")
            .and_then(|v| v.as_str())
            .unwrap_or(&task_row.task_id)
            .to_string();

        // Create metadata from task data
        let mut metadata = HashMap::new();
        metadata.insert("id".to_string(), serde_json::json!(task_row.task_id));
        metadata.insert("status".to_string(), serde_json::json!(task_row.status));

        if let Some(dod) = task_data.get("dod") {
            metadata.insert("dod".to_string(), dod.clone());
        }
        if let Some(deps) = task_data.get("dependencies") {
            metadata.insert("dependencies".to_string(), deps.clone());
        }
        if let Some(domain) = task_data.get("domain") {
            metadata.insert("domain".to_string(), domain.clone());
        }

        // Parse timestamp from updated_at
        let timestamp = task_row.updated_at
            .and_then(|s| chrono::DateTime::parse_from_rfc3339(&s).ok())
            .map(|dt| dt.timestamp_millis());

        let node_id = generate_node_id(&format!("c4task:{}", task_row.task_id));

        // Store mapping for dependency resolution
        task_node_map.insert(task_row.task_id.clone(), node_id.clone());

        if node_ids.insert(node_id.clone()) {
            nodes.push(CanvasNode {
                id: node_id,
                node_type: NodeType::Task,
                label: format!("{} {}", task_row.task_id, truncate_string(&title, 30)),
                path: None,
                metadata,
                position: Position { x: 0.0, y: 0.0 },
                timestamp,
            });
        }
    }

    // Add dependency edges between tasks
    for node in nodes.iter().filter(|n| n.node_type == NodeType::Task) {
        if let Some(deps) = node.metadata.get("dependencies") {
            if let Some(dep_array) = deps.as_array() {
                for dep in dep_array {
                    if let Some(dep_task_id) = dep.as_str() {
                        if let Some(dep_node_id) = task_node_map.get(dep_task_id) {
                            let edge_id = format!("edge_{}_{}", node.id, dep_node_id);
                            edges.push(CanvasEdge {
                                id: edge_id,
                                source: node.id.clone(),
                                target: dep_node_id.clone(),
                                relation: RelationType::Depends,
                            });
                        }
                    }
                }
            }
        }
    }

    Ok(())
}

/// Helper struct for reading C4 task rows
struct C4TaskRow {
    task_id: String,
    task_json: String,
    status: String,
    updated_at: Option<String>,
}

/// Truncate a string to a maximum length (UTF-8 safe)
///
/// Uses char_indices() to ensure we only cut at valid UTF-8 boundaries.
fn truncate_string(s: &str, max_len: usize) -> String {
    if s.len() <= max_len {
        s.to_string()
    } else {
        // Find the last valid char boundary at or before max_len
        let mut cut_pos = 0;
        for (idx, _) in s.char_indices() {
            if idx > max_len {
                break;
            }
            cut_pos = idx;
        }

        // If we found a valid position, truncate there
        if cut_pos > 0 {
            format!("{}...", &s[..cut_pos])
        } else {
            // String starts with a character longer than max_len
            "...".to_string()
        }
    }
}

// ---------------------------------------------------------------------------
// Session → Channel sync
// ---------------------------------------------------------------------------

/// Derive a slug from a project path: /Users/foo/bar -> -Users-foo-bar
fn path_to_slug_scanner(path: &str) -> String {
    let slug = path.replace('/', "-").replace('\\', "-");
    if slug.starts_with('-') {
        slug
    } else {
        format!("-{}", slug)
    }
}

/// Generate channel name from session id and creation timestamp (ms).
/// Format: `claude-{MMDD}-{session_id[..8]}`
pub fn session_channel_name(session_id: &str, created_ms: i64) -> String {
    let dt = Utc.timestamp_millis_opt(created_ms).single().unwrap_or_else(Utc::now);
    let mmdd = dt.format("%m%d").to_string();
    let id_prefix = &session_id[..session_id.len().min(8)];
    format!("claude-{}-{}", mmdd, id_prefix)
}

/// Get the creation timestamp for a session file.
/// Tries to read the `timestamp` field from the first JSON line.
/// Falls back to file creation time → file modification time.
fn session_created_ms(session_path: &Path) -> i64 {
    use std::io::{BufRead, BufReader};

    // Try first JSONL line's `timestamp` field
    if let Ok(file) = fs::File::open(session_path) {
        let mut reader = BufReader::new(file);
        let mut line = String::new();
        if reader.read_line(&mut line).is_ok() {
            if let Ok(obj) = serde_json::from_str::<serde_json::Value>(&line) {
                // Claude session lines use "timestamp" as ISO 8601 string
                if let Some(ts_str) = obj.get("timestamp").and_then(|v| v.as_str()) {
                    if let Ok(dt) = chrono::DateTime::parse_from_rfc3339(ts_str) {
                        return dt.timestamp_millis();
                    }
                }
            }
        }
    }

    // Fallback: file modification time
    fs::metadata(session_path)
        .ok()
        .and_then(|m| m.modified().ok())
        .and_then(|t| t.duration_since(SystemTime::UNIX_EPOCH).ok())
        .map(|d| d.as_millis() as i64)
        .unwrap_or(0)
}

/// Upsert a single session channel to Supabase.
/// Uses `ON CONFLICT` via PostgREST `Prefer: resolution=merge-duplicates`.
/// Returns the upserted Channel.
fn upsert_session_channel(
    client: &reqwest::blocking::Client,
    supabase_url: &str,
    anon_key: &str,
    token: &str,
    project_id: &str,
    channel_name: &str,
    session_id: &str,
    description: &str,
) -> Result<Channel, String> {
    let url = format!(
        "{}/rest/v1/c1_channels?on_conflict=project_id,name",
        supabase_url.trim_end_matches('/')
    );

    let payload = serde_json::json!({
        "project_id": project_id,
        "name": channel_name,
        "channel_type": "session",
        "created_by": session_id,
        "description": description,
    });

    let resp = retry_request(3, || {
        client
            .post(&url)
            .header("Authorization", format!("Bearer {}", token))
            .header("apikey", anon_key)
            .header("Content-Type", "application/json")
            .header("Prefer", "resolution=merge-duplicates,return=representation")
            .json(&payload)
            .send()
    })?;

    if !resp.status().is_success() {
        let status = resp.status();
        let body = resp.text().unwrap_or_default();
        return Err(format!(
            "Failed to upsert session channel '{}' ({}): {}",
            channel_name, status, body
        ));
    }

    let channels: Vec<Channel> = resp
        .json()
        .map_err(|e| format!("Failed to parse upsert response: {}", e))?;

    channels
        .into_iter()
        .next()
        .ok_or_else(|| format!("No channel returned for '{}'", channel_name))
}

/// Tauri command: scan local Claude sessions for a project and upsert
/// session-type channels to Supabase.
///
/// Name format: `claude-{MMDD}-{session_uuid_8}` where MMDD is derived from
/// the session's creation timestamp (first JSONL line or file mtime).
/// `created_by` stores the session UUID for external_id tracking.
#[tauri::command(rename_all = "camelCase")]
pub async fn sync_session_channels(
    project_id: String,
    project_path: String,
) -> Result<Vec<Channel>, String> {
    tokio::task::spawn_blocking(move || {
        let (supabase_url, anon_key) = read_supabase_config()?;
        let token = read_auth_token()?;
        let client = build_client()?;

        // Locate session directory
        let home = dirs::home_dir().ok_or("Could not find home directory")?;
        let slug = path_to_slug_scanner(&project_path);
        let sessions_dir = home.join(".claude").join("projects").join(&slug);

        if !sessions_dir.exists() {
            return Ok(Vec::new());
        }

        let mut channels = Vec::new();

        for entry in
            fs::read_dir(&sessions_dir).map_err(|e| format!("Read dir failed: {}", e))?
        {
            let entry = entry.map_err(|e| format!("Entry error: {}", e))?;
            let file_path = entry.path();

            if file_path.extension().and_then(|e| e.to_str()) != Some("jsonl") {
                continue;
            }

            let session_id = match file_path.file_stem().and_then(|n| n.to_str()) {
                Some(s) if s.len() >= 36 => s.to_string(),
                _ => continue, // skip non-UUID filenames
            };

            let created_ms = session_created_ms(&file_path);
            let channel_name = session_channel_name(&session_id, created_ms);

            // Use session title as description if available (first summary line)
            let description = {
                use std::io::{BufRead, BufReader};
                let mut desc = String::new();
                if let Ok(file) = fs::File::open(&file_path) {
                    let mut reader = BufReader::new(file);
                    let mut line = String::new();
                    for _ in 0..20 {
                        line.clear();
                        if reader.read_line(&mut line).unwrap_or(0) == 0 {
                            break;
                        }
                        if let Ok(obj) = serde_json::from_str::<serde_json::Value>(&line) {
                            if obj.get("type").and_then(|v| v.as_str()) == Some("summary") {
                                if let Some(s) = obj.get("summary").and_then(|v| v.as_str()) {
                                    desc = s.chars().take(200).collect();
                                    break;
                                }
                            }
                        }
                    }
                }
                desc
            };

            match upsert_session_channel(
                &client,
                &supabase_url,
                &anon_key,
                &token,
                &project_id,
                &channel_name,
                &session_id,
                &description,
            ) {
                Ok(ch) => channels.push(ch),
                Err(e) => {
                    // Log but continue processing remaining sessions
                    eprintln!("[sync_session_channels] skipping {}: {}", channel_name, e);
                }
            }
        }

        Ok(channels)
    })
    .await
    .map_err(|e| format!("Task execution failed: {}", e))?
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_sync_session_channels_name_format() {
        // 2026-02-22T10:30:00Z → 02-22 → mmdd = "0222"
        let session_id = "a1b2c3d4-e5f6-7890-abcd-ef1234567890";
        // 2026-02-22T10:30:00Z in millis
        let ms: i64 = 1740220200000;
        let name = session_channel_name(session_id, ms);
        assert_eq!(name, "claude-0222-a1b2c3d4");
    }

    #[test]
    fn test_session_channel_name_short_id() {
        // session_id shorter than 8 chars → use full id
        let session_id = "abc";
        let ms: i64 = 1740220200000; // 2026-02-22
        let name = session_channel_name(session_id, ms);
        assert_eq!(name, "claude-0222-abc");
    }

    #[test]
    fn test_session_channel_name_zero_timestamp() {
        // zero ms → 1970-01-01 → mmdd = "0101"
        let session_id = "deadbeef-0000-0000-0000-000000000000";
        let name = session_channel_name(session_id, 0);
        assert_eq!(name, "claude-0101-deadbeef");
    }

    #[test]
    fn test_truncate_string_ascii() {
        assert_eq!(truncate_string("hello world", 5), "hello...");
        assert_eq!(truncate_string("hello", 5), "hello");
        assert_eq!(truncate_string("hello", 10), "hello");
    }

    #[test]
    fn test_truncate_string_korean() {
        // Korean text: "task_ops.py 모델 라우팅 적용"
        let korean_text = "task_ops.py 모델 라우팅 적용";
        let result = truncate_string(korean_text, 30);

        // Should not panic and should be valid UTF-8
        assert!(result.len() <= 33); // 30 + "..." (3 bytes)
        assert!(std::str::from_utf8(result.as_bytes()).is_ok());
    }

    #[test]
    fn test_truncate_string_emoji() {
        // Emoji are multi-byte UTF-8 characters
        let emoji_text = "Hello 👋 World 🌍";
        let result = truncate_string(emoji_text, 10);

        // Should not panic and should be valid UTF-8
        assert!(std::str::from_utf8(result.as_bytes()).is_ok());
    }

    #[test]
    fn test_truncate_string_empty() {
        assert_eq!(truncate_string("", 10), "");
        assert_eq!(truncate_string("", 0), "");
    }

    #[test]
    fn test_truncate_string_exact_length() {
        let text = "exactly10!";
        assert_eq!(truncate_string(text, 10), "exactly10!");
    }

    #[test]
    fn test_get_project_id_from_config() {
        // Test with actual project root
        let project_root = Path::new("/Users/changmin/git/cq");
        let result = get_project_id(project_root);

        assert!(result.is_ok());
        let project_id = result.unwrap();

        // Should read from config.yaml (which has project_id: c4)
        assert_eq!(project_id, "c4");
    }

    #[test]
    fn test_get_project_id_fallback_to_dirname() {
        use std::env;

        // Create a temporary directory without .c4/config.yaml
        let temp_dir = env::temp_dir().join("test_project_fallback");
        std::fs::create_dir_all(&temp_dir).ok();

        let result = get_project_id(&temp_dir);

        // Clean up
        std::fs::remove_dir_all(&temp_dir).ok();

        assert!(result.is_ok());
        let project_id = result.unwrap();

        // Should fall back to directory name
        assert_eq!(project_id, "test_project_fallback");
    }

    #[test]
    fn test_get_project_id_with_invalid_yaml() {
        use std::env;
        use std::fs;

        // Create a temporary directory with invalid YAML
        let temp_dir = env::temp_dir().join("test_project_invalid_yaml");
        let c4_dir = temp_dir.join(".c4");
        fs::create_dir_all(&c4_dir).ok();

        // Write invalid YAML
        let config_path = c4_dir.join("config.yaml");
        fs::write(&config_path, "invalid: yaml: content: [[[").ok();

        let result = get_project_id(&temp_dir);

        // Clean up
        fs::remove_dir_all(&temp_dir).ok();

        assert!(result.is_ok());
        let project_id = result.unwrap();

        // Should fall back to directory name
        assert_eq!(project_id, "test_project_invalid_yaml");
    }
}
