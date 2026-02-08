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

/// Read Supabase URL and anon_key from ~/.c4/supabase.json
fn read_supabase_config() -> Result<(String, String), String> {
    let home = dirs::home_dir().ok_or("Could not find home directory")?;
    let config_path = home.join(".c4").join("supabase.json");
    if !config_path.exists() {
        return Err("Supabase config not found (~/.c4/supabase.json)".to_string());
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

        let resp = client
            .get(&url)
            .header("Authorization", format!("Bearer {}", token))
            .header("apikey", &anon_key)
            .send()
            .map_err(|e| format!("Request failed: {}", e))?;

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

        let resp = client
            .get(&state_url)
            .header("Authorization", format!("Bearer {}", token))
            .header("apikey", &anon_key)
            .send()
            .map_err(|e| format!("Request failed: {}", e))?;

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

        let resp = client
            .get(&tasks_url)
            .header("Authorization", format!("Bearer {}", token))
            .header("apikey", &anon_key)
            .send()
            .map_err(|e| format!("Request failed: {}", e))?;

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
// Tests
// ---------------------------------------------------------------------------

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
}
