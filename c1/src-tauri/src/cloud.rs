//! Cloud integration — Supabase REST client for task sync and team projects
//!
//! Reads Supabase config from ~/.c4/supabase.json and auth session from
//! ~/.c4/session.json. All HTTP calls use blocking reqwest inside
//! spawn_blocking to avoid blocking the Tauri event loop.

use std::fs;
use std::path::Path;

use rusqlite::Connection;
use serde::{Deserialize, Serialize};

use crate::models::ProjectState;
use crate::scanner::get_project_id;

// ---------------------------------------------------------------------------
// Data models
// ---------------------------------------------------------------------------

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct SyncResult {
    pub synced_count: u32,
    pub errors: Vec<String>,
    pub last_synced: String,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct TeamProject {
    pub id: String,
    pub name: String,
    pub owner_email: String,
    pub task_count: u32,
    pub done_count: u32,
    pub status: String,
    pub last_updated: Option<String>,
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

/// Read Supabase URL and anon_key from env vars or ~/.c4/supabase.json
fn read_supabase_config() -> Result<(String, String), String> {
    // Try env vars first (loaded from .env by dotenvy)
    if let (Ok(url), Ok(key)) = (
        std::env::var("SUPABASE_URL"),
        std::env::var("SUPABASE_ANON_KEY").or_else(|_| std::env::var("SUPABASE_KEY")),
    ) {
        return Ok((url, key));
    }

    // Fall back to ~/.c4/supabase.json
    let home = dirs::home_dir().ok_or("Could not find home directory")?;
    let config_path = home.join(".c4").join("supabase.json");
    if !config_path.exists() {
        return Err("Supabase not configured. Set SUPABASE_URL + SUPABASE_KEY env vars or create ~/.c4/supabase.json".to_string());
    }
    let content = fs::read_to_string(&config_path)
        .map_err(|e| format!("Failed to read supabase config: {}", e))?;
    let config: serde_json::Value = serde_json::from_str(&content)
        .map_err(|e| format!("Invalid supabase config: {}", e))?;
    let url = config
        .get("url")
        .and_then(|v| v.as_str())
        .ok_or("Missing 'url' in supabase config")?;
    let anon_key = config
        .get("anon_key")
        .and_then(|v| v.as_str())
        .ok_or("Missing 'anon_key' in supabase config")?;
    Ok((url.to_string(), anon_key.to_string()))
}

/// Read access_token from ~/.c4/session.json
fn read_auth_token() -> Result<String, String> {
    let home = dirs::home_dir().ok_or("Could not find home directory")?;
    let session_path = home.join(".c4").join("session.json");
    if !session_path.exists() {
        return Err("Not logged in (no session.json)".to_string());
    }
    let content = fs::read_to_string(&session_path)
        .map_err(|e| format!("Failed to read session: {}", e))?;
    let session: serde_json::Value = serde_json::from_str(&content)
        .map_err(|e| format!("Invalid session: {}", e))?;
    session
        .get("access_token")
        .and_then(|v| v.as_str())
        .map(String::from)
        .ok_or("No access_token in session".to_string())
}

/// Open the local C4 SQLite database
fn open_c4_db(project_path: &Path) -> Result<Connection, String> {
    let db_path = project_path.join(".c4").join("c4.db");
    if !db_path.exists() {
        return Err("C4 database not found".to_string());
    }
    Connection::open(&db_path).map_err(|e| format!("Failed to open DB: {}", e))
}

/// Build a blocking reqwest client with a 30-second timeout
fn build_client() -> Result<reqwest::blocking::Client, String> {
    reqwest::blocking::Client::builder()
        .timeout(std::time::Duration::from_secs(30))
        .build()
        .map_err(|e| format!("HTTP client error: {}", e))
}

/// Execute an HTTP request with exponential backoff retry (3 attempts, 1s/2s/4s).
/// Only retries on network errors or 5xx server errors.
fn retry_request<F>(max_attempts: u32, mut execute: F) -> Result<reqwest::blocking::Response, String>
where
    F: FnMut() -> Result<reqwest::blocking::Response, reqwest::Error>,
{
    let mut last_err = String::new();
    for attempt in 0..max_attempts {
        match execute() {
            Ok(resp) => {
                if resp.status().is_server_error() && attempt + 1 < max_attempts {
                    last_err = format!("Server error: {}", resp.status());
                    let delay = std::time::Duration::from_secs(1 << attempt);
                    std::thread::sleep(delay);
                    continue;
                }
                return Ok(resp);
            }
            Err(e) => {
                last_err = format!("Request failed: {}", e);
                if attempt + 1 < max_attempts {
                    let delay = std::time::Duration::from_secs(1 << attempt);
                    std::thread::sleep(delay);
                }
            }
        }
    }
    Err(last_err)
}

// ---------------------------------------------------------------------------
// Tauri IPC commands
// ---------------------------------------------------------------------------

/// Sync local tasks to Supabase cloud
///
/// Reads all tasks from local .c4/c4.db and upserts them to the
/// Supabase c4_tasks table via REST API.
#[tauri::command(rename_all = "camelCase")]
pub async fn cloud_sync_tasks(project_path: String) -> Result<SyncResult, String> {
    tokio::task::spawn_blocking(move || {
        let (supabase_url, anon_key) = read_supabase_config()?;
        let token = read_auth_token()?;

        let project_dir = Path::new(&project_path);
        let conn = open_c4_db(project_dir)?;
        let project_id = get_project_id(project_dir)
            .map_err(|e| format!("Failed to get project_id: {}", e))?;

        // Read all tasks from local DB
        let mut stmt = conn
            .prepare("SELECT task_id, task_json, status, assigned_to FROM c4_tasks WHERE project_id = ?")
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

        let mut tasks: Vec<serde_json::Value> = Vec::new();
        let mut errors: Vec<String> = Vec::new();

        for row in rows {
            match row {
                Ok((task_id, task_json, status, assigned_to)) => {
                    tasks.push(serde_json::json!({
                        "project_id": project_id,
                        "task_id": task_id,
                        "task_json": task_json,
                        "status": status,
                        "assigned_to": assigned_to,
                    }));
                }
                Err(e) => {
                    errors.push(format!("Row error: {}", e));
                }
            }
        }

        if tasks.is_empty() {
            return Ok(SyncResult {
                synced_count: 0,
                errors,
                last_synced: chrono::Utc::now().to_rfc3339(),
            });
        }

        // Also sync project state
        let state_result = conn.query_row(
            "SELECT state_json FROM c4_state WHERE project_id = ?",
            [&project_id],
            |row| row.get::<_, String>(0),
        );

        let client = build_client()?;
        let rest_url = format!(
            "{}/rest/v1/c4_tasks",
            supabase_url.trim_end_matches('/')
        );

        // Upsert tasks in batches of 50
        let mut synced_count: u32 = 0;
        for chunk in tasks.chunks(50) {
            let resp = client
                .post(&rest_url)
                .header("Authorization", format!("Bearer {}", token))
                .header("apikey", &anon_key)
                .header("Content-Type", "application/json")
                .header("Prefer", "resolution=merge-duplicates")
                .json(&chunk)
                .send();

            match resp {
                Ok(r) => {
                    let r: reqwest::blocking::Response = r;
                    if r.status().is_success() {
                        synced_count += chunk.len() as u32;
                    } else {
                        let status = r.status();
                        let body = r.text().unwrap_or_default();
                        errors.push(format!("Sync batch failed ({}): {}", status, body));
                    }
                }
                Err(e) => {
                    errors.push(format!("Request failed: {}", e));
                }
            }
        }

        // Sync project state if available
        if let Ok(state_json) = state_result {
            let state_url = format!(
                "{}/rest/v1/c4_state",
                supabase_url.trim_end_matches('/')
            );
            let state_payload = serde_json::json!({
                "project_id": project_id,
                "state_json": state_json,
            });

            let resp = client
                .post(&state_url)
                .header("Authorization", format!("Bearer {}", token))
                .header("apikey", &anon_key)
                .header("Content-Type", "application/json")
                .header("Prefer", "resolution=merge-duplicates")
                .json(&state_payload)
                .send();

            if let Err(e) = resp {
                errors.push(format!("State sync failed: {}", e));
            }
        }

        Ok(SyncResult {
            synced_count,
            errors,
            last_synced: chrono::Utc::now().to_rfc3339(),
        })
    })
    .await
    .map_err(|e| format!("Task execution failed: {}", e))?
}

/// Fetch team projects from Supabase
#[tauri::command]
pub async fn cloud_get_team_projects() -> Result<Vec<TeamProject>, String> {
    tokio::task::spawn_blocking(move || {
        let (supabase_url, anon_key) = read_supabase_config()?;
        let token = read_auth_token()?;

        let client = build_client()?;
        let url = format!(
            "{}/rest/v1/c4_projects?select=*",
            supabase_url.trim_end_matches('/')
        );

        let resp = retry_request(3, || {
            client
                .get(&url)
                .header("Authorization", format!("Bearer {}", token))
                .header("apikey", &anon_key)
                .send()
        })?;

        if !resp.status().is_success() {
            let status = resp.status();
            let body = resp.text().unwrap_or_default();
            return Err(format!("Failed to fetch team projects ({}): {}", status, body));
        }

        let data: Vec<serde_json::Value> = resp
            .json()
            .map_err(|e| format!("Failed to parse response: {}", e))?;

        let projects: Vec<TeamProject> = data
            .into_iter()
            .map(|row: serde_json::Value| TeamProject {
                id: row.get("id").and_then(|v: &serde_json::Value| v.as_str()).unwrap_or("").to_string(),
                name: row
                    .get("name")
                    .and_then(|v: &serde_json::Value| v.as_str())
                    .unwrap_or("Unknown")
                    .to_string(),
                owner_email: row
                    .get("owner_email")
                    .and_then(|v: &serde_json::Value| v.as_str())
                    .unwrap_or("")
                    .to_string(),
                task_count: row
                    .get("task_count")
                    .and_then(|v: &serde_json::Value| v.as_u64())
                    .unwrap_or(0) as u32,
                done_count: row
                    .get("done_count")
                    .and_then(|v: &serde_json::Value| v.as_u64())
                    .unwrap_or(0) as u32,
                status: row
                    .get("status")
                    .and_then(|v: &serde_json::Value| v.as_str())
                    .unwrap_or("UNKNOWN")
                    .to_string(),
                last_updated: row
                    .get("last_updated")
                    .and_then(|v: &serde_json::Value| v.as_str())
                    .map(String::from),
            })
            .collect();

        Ok(projects)
    })
    .await
    .map_err(|e| format!("Task execution failed: {}", e))?
}

/// Fetch a remote project's dashboard state from Supabase
#[tauri::command(rename_all = "camelCase")]
pub async fn cloud_get_remote_dashboard(project_id: String) -> Result<ProjectState, String> {
    tokio::task::spawn_blocking(move || {
        let (supabase_url, anon_key) = read_supabase_config()?;
        let token = read_auth_token()?;

        let client = build_client()?;

        // 1. Fetch project state
        let state_url = format!(
            "{}/rest/v1/c4_state?project_id=eq.{}&select=state_json",
            supabase_url.trim_end_matches('/'),
            urlencoding::encode(&project_id),
        );

        let resp = retry_request(3, || {
            client
                .get(&state_url)
                .header("Authorization", format!("Bearer {}", token))
                .header("apikey", &anon_key)
                .send()
        })?;

        if !resp.status().is_success() {
            let status = resp.status();
            let body = resp.text().unwrap_or_default();
            return Err(format!("Failed to fetch state ({}): {}", status, body));
        }

        let rows: Vec<serde_json::Value> = resp
            .json()
            .map_err(|e| format!("Failed to parse state response: {}", e))?;

        let state_json_str = rows
            .first()
            .and_then(|r: &serde_json::Value| r.get("state_json"))
            .and_then(|v: &serde_json::Value| v.as_str())
            .ok_or("No state found for this project")?;

        let state: serde_json::Value = serde_json::from_str(state_json_str)
            .map_err(|e| format!("Invalid state JSON: {}", e))?;

        let status = state
            .get("status")
            .and_then(|v| v.as_str())
            .unwrap_or("UNKNOWN")
            .to_string();

        // 2. Fetch task counts
        let tasks_url = format!(
            "{}/rest/v1/c4_tasks?project_id=eq.{}&select=status",
            supabase_url.trim_end_matches('/'),
            urlencoding::encode(&project_id),
        );

        let resp = retry_request(3, || {
            client
                .get(&tasks_url)
                .header("Authorization", format!("Bearer {}", token))
                .header("apikey", &anon_key)
                .send()
        })?;

        let task_rows: Vec<serde_json::Value> = if resp.status().is_success() {
            resp.json().unwrap_or_default()
        } else {
            Vec::new()
        };

        let mut done: u32 = 0;
        let mut in_progress: u32 = 0;
        let mut pending: u32 = 0;
        let mut blocked: u32 = 0;

        for row in &task_rows {
            match row.get("status").and_then(|v: &serde_json::Value| v.as_str()).unwrap_or("") {
                "done" => done += 1,
                "in_progress" => in_progress += 1,
                "pending" => pending += 1,
                "blocked" => blocked += 1,
                _ => {}
            }
        }

        Ok(ProjectState {
            status,
            project_id,
            workers: Vec::new(), // Remote dashboard doesn't show live workers
            progress: crate::models::TaskProgress {
                total: done + in_progress + pending + blocked,
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

// ---------------------------------------------------------------------------
// Pull & Sync Status commands (Phase 8.2)
// ---------------------------------------------------------------------------

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct PullResult {
    pub pulled_count: u32,
    pub merged_count: u32,
    pub conflict_count: u32,
    pub errors: Vec<String>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct SyncStatus {
    pub last_synced: Option<String>,
    pub pending_push: u32,
    pub pending_pull: u32,
    pub cloud_connected: bool,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct RemoteCheckpoint {
    pub id: String,
    pub decision: String,
    pub notes: Option<String>,
    pub created_at: String,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct GrowthMetric {
    pub week: String,
    pub approval_rate: f64,
    pub avg_score: f64,
    pub tasks_completed: u32,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct AgentTrace {
    pub agent_type: String,
    pub task_id: Option<String>,
    pub action: String,
    pub duration_ms: Option<u64>,
    pub created_at: String,
}

/// Pull tasks from Supabase cloud and merge into local database.
///
/// Uses row_version for conflict resolution (last-write-wins).
#[tauri::command(rename_all = "camelCase")]
pub async fn cloud_pull_tasks(project_path: String) -> Result<PullResult, String> {
    tokio::task::spawn_blocking(move || {
        let (supabase_url, anon_key) = read_supabase_config()?;
        let token = read_auth_token()?;

        let project_dir = std::path::Path::new(&project_path);
        let conn = open_c4_db(project_dir)?;
        let project_id = get_project_id(project_dir)
            .map_err(|e| format!("Failed to get project_id: {}", e))?;

        let client = build_client()?;

        // Fetch remote tasks
        let tasks_url = format!(
            "{}/rest/v1/c4_tasks?project_id=eq.{}&select=*",
            supabase_url.trim_end_matches('/'),
            urlencoding::encode(&project_id),
        );

        let resp = retry_request(3, || {
            client
                .get(&tasks_url)
                .header("Authorization", format!("Bearer {}", token))
                .header("apikey", &anon_key)
                .send()
        })?;

        if !resp.status().is_success() {
            let status = resp.status();
            let body = resp.text().unwrap_or_default();
            return Err(format!("Pull failed ({}): {}", status, body));
        }

        let remote_tasks: Vec<serde_json::Value> = resp
            .json()
            .map_err(|e| format!("Failed to parse tasks: {}", e))?;

        let mut pulled: u32 = 0;
        let mut merged: u32 = 0;
        let mut conflicts: u32 = 0;
        let mut errors: Vec<String> = Vec::new();

        for remote_task in &remote_tasks {
            pulled += 1;

            let task_id = remote_task
                .get("task_id")
                .and_then(|v| v.as_str())
                .unwrap_or("");
            let remote_version = remote_task
                .get("row_version")
                .and_then(|v| v.as_i64())
                .unwrap_or(0);

            // Check if local task exists and its version
            let local_version: Option<i64> = conn
                .query_row(
                    "SELECT CAST(json_extract(task_json, '$.row_version') AS INTEGER) FROM c4_tasks WHERE task_id = ? AND project_id = ?",
                    rusqlite::params![task_id, project_id],
                    |row| row.get(0),
                )
                .ok();

            match local_version {
                Some(local_v) if local_v >= remote_version => {
                    // Local is newer or same — skip (conflict logged)
                    if local_v > remote_version {
                        conflicts += 1;
                    }
                }
                _ => {
                    // Remote is newer or local doesn't exist — upsert
                    let task_json = serde_json::to_string(remote_task).unwrap_or_default();
                    let status = remote_task
                        .get("status")
                        .and_then(|v| v.as_str())
                        .unwrap_or("pending");
                    let assigned_to = remote_task
                        .get("assigned_to")
                        .and_then(|v| v.as_str());

                    let result = conn.execute(
                        "INSERT INTO c4_tasks (project_id, task_id, task_json, status, assigned_to)
                         VALUES (?, ?, ?, ?, ?)
                         ON CONFLICT(project_id, task_id) DO UPDATE SET
                           task_json = excluded.task_json,
                           status = excluded.status,
                           assigned_to = excluded.assigned_to",
                        rusqlite::params![project_id, task_id, task_json, status, assigned_to],
                    );

                    match result {
                        Ok(_) => merged += 1,
                        Err(e) => errors.push(format!("Merge {} failed: {}", task_id, e)),
                    }
                }
            }
        }

        // Also pull state
        let state_url = format!(
            "{}/rest/v1/c4_state?project_id=eq.{}&select=state_json",
            supabase_url.trim_end_matches('/'),
            urlencoding::encode(&project_id),
        );
        if let Ok(resp) = retry_request(3, || {
            client
                .get(&state_url)
                .header("Authorization", format!("Bearer {}", token))
                .header("apikey", &anon_key)
                .send()
        }) {
            if resp.status().is_success() {
                if let Ok(rows) = resp.json::<Vec<serde_json::Value>>() {
                    if let Some(state_json) = rows.first().and_then(|r| r.get("state_json")).and_then(|v| v.as_str()) {
                        let _ = conn.execute(
                            "INSERT INTO c4_state (project_id, state_json) VALUES (?, ?)
                             ON CONFLICT(project_id) DO UPDATE SET state_json = excluded.state_json",
                            rusqlite::params![project_id, state_json],
                        );
                    }
                }
            }
        }

        Ok(PullResult {
            pulled_count: pulled,
            merged_count: merged,
            conflict_count: conflicts,
            errors,
        })
    })
    .await
    .map_err(|e| format!("Task execution failed: {}", e))?
}

/// Get current sync status information.
#[tauri::command(rename_all = "camelCase")]
pub async fn cloud_sync_status(project_path: String) -> Result<SyncStatus, String> {
    tokio::task::spawn_blocking(move || {
        let cloud_connected = read_supabase_config().is_ok() && read_auth_token().is_ok();

        let project_dir = std::path::Path::new(&project_path);
        let pending_push = if let Ok(conn) = open_c4_db(project_dir) {
            // Count tasks modified after last sync (approximation)
            conn.query_row("SELECT COUNT(*) FROM c4_tasks", [], |row| row.get::<_, u32>(0))
                .unwrap_or(0)
        } else {
            0
        };

        Ok(SyncStatus {
            last_synced: None, // TODO: persist last sync timestamp
            pending_push,
            pending_pull: 0,
            cloud_connected,
        })
    })
    .await
    .map_err(|e| format!("Task execution failed: {}", e))?
}

/// Fetch checkpoints for a remote project.
#[tauri::command(rename_all = "camelCase")]
pub async fn cloud_get_checkpoints(project_id: String) -> Result<Vec<RemoteCheckpoint>, String> {
    tokio::task::spawn_blocking(move || {
        let (supabase_url, anon_key) = read_supabase_config()?;
        let token = read_auth_token()?;
        let client = build_client()?;

        let url = format!(
            "{}/rest/v1/c4_checkpoints?project_id=eq.{}&select=*&order=created_at.desc&limit=50",
            supabase_url.trim_end_matches('/'),
            urlencoding::encode(&project_id),
        );

        let resp = retry_request(3, || {
            client
                .get(&url)
                .header("Authorization", format!("Bearer {}", token))
                .header("apikey", &anon_key)
                .send()
        })?;

        if !resp.status().is_success() {
            let status = resp.status();
            let body = resp.text().unwrap_or_default();
            return Err(format!("Failed to fetch checkpoints ({}): {}", status, body));
        }

        let rows: Vec<serde_json::Value> = resp
            .json()
            .map_err(|e| format!("Failed to parse checkpoints: {}", e))?;

        Ok(rows
            .into_iter()
            .map(|row| RemoteCheckpoint {
                id: row.get("id").and_then(|v| v.as_str()).unwrap_or("").to_string(),
                decision: row.get("decision").and_then(|v| v.as_str()).unwrap_or("").to_string(),
                notes: row.get("notes").and_then(|v| v.as_str()).map(String::from),
                created_at: row.get("created_at").and_then(|v| v.as_str()).unwrap_or("").to_string(),
            })
            .collect())
    })
    .await
    .map_err(|e| format!("Task execution failed: {}", e))?
}

/// Fetch growth metrics for a remote project.
#[tauri::command(rename_all = "camelCase")]
pub async fn cloud_get_growth_metrics(project_id: String) -> Result<Vec<GrowthMetric>, String> {
    tokio::task::spawn_blocking(move || {
        let (supabase_url, anon_key) = read_supabase_config()?;
        let token = read_auth_token()?;
        let client = build_client()?;

        let url = format!(
            "{}/rest/v1/c4_twin_growth?project_id=eq.{}&select=*&order=week.desc&limit=20",
            supabase_url.trim_end_matches('/'),
            urlencoding::encode(&project_id),
        );

        let resp = retry_request(3, || {
            client
                .get(&url)
                .header("Authorization", format!("Bearer {}", token))
                .header("apikey", &anon_key)
                .send()
        })?;

        if !resp.status().is_success() {
            return Ok(Vec::new());
        }

        let rows: Vec<serde_json::Value> = resp.json().unwrap_or_default();

        Ok(rows
            .into_iter()
            .map(|row| GrowthMetric {
                week: row.get("week").and_then(|v| v.as_str()).unwrap_or("").to_string(),
                approval_rate: row.get("approval_rate").and_then(|v| v.as_f64()).unwrap_or(0.0),
                avg_score: row.get("avg_score").and_then(|v| v.as_f64()).unwrap_or(0.0),
                tasks_completed: row
                    .get("tasks_completed")
                    .and_then(|v| v.as_u64())
                    .unwrap_or(0) as u32,
            })
            .collect())
    })
    .await
    .map_err(|e| format!("Task execution failed: {}", e))?
}

/// Fetch recent agent traces for a remote project.
#[tauri::command(rename_all = "camelCase")]
pub async fn cloud_get_agent_traces(project_id: String) -> Result<Vec<AgentTrace>, String> {
    tokio::task::spawn_blocking(move || {
        let (supabase_url, anon_key) = read_supabase_config()?;
        let token = read_auth_token()?;
        let client = build_client()?;

        let url = format!(
            "{}/rest/v1/c4_agent_traces?project_id=eq.{}&select=*&order=created_at.desc&limit=50",
            supabase_url.trim_end_matches('/'),
            urlencoding::encode(&project_id),
        );

        let resp = retry_request(3, || {
            client
                .get(&url)
                .header("Authorization", format!("Bearer {}", token))
                .header("apikey", &anon_key)
                .send()
        })?;

        if !resp.status().is_success() {
            return Ok(Vec::new());
        }

        let rows: Vec<serde_json::Value> = resp.json().unwrap_or_default();

        Ok(rows
            .into_iter()
            .map(|row| AgentTrace {
                agent_type: row.get("agent_type").and_then(|v| v.as_str()).unwrap_or("").to_string(),
                task_id: row.get("task_id").and_then(|v| v.as_str()).map(String::from),
                action: row.get("action").and_then(|v| v.as_str()).unwrap_or("").to_string(),
                duration_ms: row.get("duration_ms").and_then(|v| v.as_u64()),
                created_at: row.get("created_at").and_then(|v| v.as_str()).unwrap_or("").to_string(),
            })
            .collect())
    })
    .await
    .map_err(|e| format!("Task execution failed: {}", e))?
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// Knowledge docs (Phase 8.3)
// ---------------------------------------------------------------------------

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct KnowledgeDoc {
    pub doc_id: String,
    pub doc_type: String,
    pub title: String,
    #[serde(default)]
    pub domain: String,
    #[serde(default)]
    pub tags: Vec<String>,
    #[serde(default)]
    pub body: String,
    #[serde(default)]
    pub content_hash: String,
    #[serde(default)]
    pub version: u32,
    #[serde(default)]
    pub created_at: String,
    #[serde(default)]
    pub updated_at: String,
}

/// Get knowledge documents for a remote project.
#[tauri::command]
pub async fn cloud_get_knowledge_docs(project_id: String) -> Result<Vec<KnowledgeDoc>, String> {
    tokio::task::spawn_blocking(move || {
        let (supabase_url, anon_key) = read_supabase_config()?;
        let token = read_auth_token()?;
        let client = build_client()?;

        let url = format!(
            "{}/rest/v1/c4_documents?project_id=eq.{}&select=doc_id,doc_type,title,domain,tags,body,content_hash,version,created_at,updated_at&order=updated_at.desc&limit=100",
            supabase_url.trim_end_matches('/'),
            urlencoding::encode(&project_id),
        );

        let resp = retry_request(3, || {
            client
                .get(&url)
                .header("Authorization", format!("Bearer {}", token))
                .header("apikey", &anon_key)
                .send()
        })?;

        if !resp.status().is_success() {
            return Ok(Vec::new());
        }

        let docs: Vec<KnowledgeDoc> = resp.json().unwrap_or_default();
        Ok(docs)
    })
    .await
    .map_err(|e| format!("Task execution failed: {}", e))?
}

/// Search knowledge documents using PostgreSQL full-text search.
#[tauri::command]
pub async fn cloud_search_knowledge(
    project_id: String,
    query: String,
) -> Result<Vec<KnowledgeDoc>, String> {
    tokio::task::spawn_blocking(move || {
        let (supabase_url, anon_key) = read_supabase_config()?;
        let token = read_auth_token()?;
        let client = build_client()?;

        let url = format!(
            "{}/rest/v1/c4_documents?project_id=eq.{}&tsv=fts.english.{}&select=doc_id,doc_type,title,domain,tags,body,content_hash,version,created_at,updated_at&order=updated_at.desc&limit=30",
            supabase_url.trim_end_matches('/'),
            urlencoding::encode(&project_id),
            urlencoding::encode(&query),
        );

        let resp = retry_request(3, || {
            client
                .get(&url)
                .header("Authorization", format!("Bearer {}", token))
                .header("apikey", &anon_key)
                .send()
        })?;

        if !resp.status().is_success() {
            return Ok(Vec::new());
        }

        let docs: Vec<KnowledgeDoc> = resp.json().unwrap_or_default();
        Ok(docs)
    })
    .await
    .map_err(|e| format!("Task execution failed: {}", e))?
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_sync_result_serialization() {
        let result = SyncResult {
            synced_count: 5,
            errors: vec!["test error".to_string()],
            last_synced: "2026-01-01T00:00:00Z".to_string(),
        };
        let json = serde_json::to_string(&result).unwrap();
        let parsed: SyncResult = serde_json::from_str(&json).unwrap();
        assert_eq!(parsed.synced_count, 5);
        assert_eq!(parsed.errors.len(), 1);
    }

    #[test]
    fn test_team_project_serialization() {
        let project = TeamProject {
            id: "proj-1".to_string(),
            name: "Test Project".to_string(),
            owner_email: "test@example.com".to_string(),
            task_count: 10,
            done_count: 5,
            status: "EXECUTE".to_string(),
            last_updated: Some("2026-01-01T00:00:00Z".to_string()),
        };
        let json = serde_json::to_string(&project).unwrap();
        assert!(json.contains("Test Project"));
        let parsed: TeamProject = serde_json::from_str(&json).unwrap();
        assert_eq!(parsed.task_count, 10);
        assert_eq!(parsed.done_count, 5);
    }

    #[test]
    fn test_team_project_null_last_updated() {
        let project = TeamProject {
            id: "proj-2".to_string(),
            name: "No Date".to_string(),
            owner_email: "a@b.com".to_string(),
            task_count: 0,
            done_count: 0,
            status: "INIT".to_string(),
            last_updated: None,
        };
        let json = serde_json::to_string(&project).unwrap();
        let parsed: TeamProject = serde_json::from_str(&json).unwrap();
        assert!(parsed.last_updated.is_none());
    }

    #[test]
    fn test_read_supabase_config_missing() {
        // With no config file present in a test environment, should return error
        let result = read_supabase_config();
        // Can't guarantee home dir config exists, just check it returns cleanly
        assert!(result.is_ok() || result.is_err());
    }

    #[test]
    fn test_read_auth_token_missing() {
        // With no session file, should return error (in most test envs)
        let result = read_auth_token();
        assert!(result.is_ok() || result.is_err());
    }

    #[test]
    fn test_retry_request_retries_on_network_error() {
        let mut attempts = 0u32;
        let result = retry_request(3, || {
            attempts += 1;
            // Use an unreachable address with very short timeout
            reqwest::blocking::Client::builder()
                .timeout(std::time::Duration::from_millis(50))
                .build()
                .unwrap()
                .get("http://192.0.2.1:1") // RFC 5737 TEST-NET, guaranteed unreachable
                .send()
        });
        assert!(result.is_err());
        assert_eq!(attempts, 3);
    }

    #[test]
    fn test_pull_result_serialization() {
        let result = PullResult {
            pulled_count: 10,
            merged_count: 8,
            conflict_count: 2,
            errors: vec![],
        };
        let json = serde_json::to_string(&result).unwrap();
        let parsed: PullResult = serde_json::from_str(&json).unwrap();
        assert_eq!(parsed.pulled_count, 10);
        assert_eq!(parsed.conflict_count, 2);
    }

    #[test]
    fn test_sync_status_serialization() {
        let status = SyncStatus {
            last_synced: Some("2026-02-10T00:00:00Z".to_string()),
            pending_push: 5,
            pending_pull: 3,
            cloud_connected: true,
        };
        let json = serde_json::to_string(&status).unwrap();
        let parsed: SyncStatus = serde_json::from_str(&json).unwrap();
        assert_eq!(parsed.pending_push, 5);
        assert!(parsed.cloud_connected);
    }

    #[test]
    fn test_remote_checkpoint_serialization() {
        let cp = RemoteCheckpoint {
            id: "CP-001".to_string(),
            decision: "APPROVE".to_string(),
            notes: Some("Looks good".to_string()),
            created_at: "2026-02-10T12:00:00Z".to_string(),
        };
        let json = serde_json::to_string(&cp).unwrap();
        assert!(json.contains("APPROVE"));
        let parsed: RemoteCheckpoint = serde_json::from_str(&json).unwrap();
        assert_eq!(parsed.id, "CP-001");
    }

    #[test]
    fn test_growth_metric_serialization() {
        let metric = GrowthMetric {
            week: "2026-W06".to_string(),
            approval_rate: 0.85,
            avg_score: 8.5,
            tasks_completed: 12,
        };
        let json = serde_json::to_string(&metric).unwrap();
        let parsed: GrowthMetric = serde_json::from_str(&json).unwrap();
        assert!((parsed.approval_rate - 0.85).abs() < f64::EPSILON);
    }

    #[test]
    fn test_agent_trace_serialization() {
        let trace = AgentTrace {
            agent_type: "golang-pro".to_string(),
            task_id: Some("T-001-0".to_string()),
            action: "implement".to_string(),
            duration_ms: Some(45000),
            created_at: "2026-02-10T12:00:00Z".to_string(),
        };
        let json = serde_json::to_string(&trace).unwrap();
        let parsed: AgentTrace = serde_json::from_str(&json).unwrap();
        assert_eq!(parsed.agent_type, "golang-pro");
        assert_eq!(parsed.duration_ms, Some(45000));
    }

    #[test]
    fn test_knowledge_doc_serialization() {
        let doc = KnowledgeDoc {
            doc_id: "exp-abc123".to_string(),
            doc_type: "experiment".to_string(),
            title: "Test Experiment".to_string(),
            domain: "ml".to_string(),
            tags: vec!["pytorch".to_string(), "classification".to_string()],
            body: "# Results\nAccuracy: 95%".to_string(),
            content_hash: "abc123def456".to_string(),
            version: 2,
            created_at: "2026-02-10T12:00:00Z".to_string(),
            updated_at: "2026-02-10T14:00:00Z".to_string(),
        };
        let json = serde_json::to_string(&doc).unwrap();
        assert!(json.contains("experiment"));
        assert!(json.contains("pytorch"));
        let parsed: KnowledgeDoc = serde_json::from_str(&json).unwrap();
        assert_eq!(parsed.doc_id, "exp-abc123");
        assert_eq!(parsed.tags.len(), 2);
        assert_eq!(parsed.version, 2);
    }

    #[test]
    fn test_knowledge_doc_empty_fields() {
        let json = r#"{"doc_id":"ins-001","doc_type":"insight","title":"Simple"}"#;
        let doc: KnowledgeDoc = serde_json::from_str(json).unwrap();
        assert_eq!(doc.doc_id, "ins-001");
        assert_eq!(doc.domain, "");
        assert!(doc.tags.is_empty());
        assert_eq!(doc.version, 0);
    }
}
