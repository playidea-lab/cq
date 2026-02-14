//! Supabase Realtime WebSocket client
//!
//! Connects to Supabase Realtime v2 (Phoenix Channels protocol) and emits
//! Tauri events when database changes occur on subscribed tables.
//!
//! Usage: call `realtime_connect` from the frontend to start the connection,
//! then listen for "cloud-update" events. Call `realtime_disconnect` to stop.

use std::sync::Arc;
use std::time::Duration;

use futures_util::{SinkExt, StreamExt};
use serde::{Deserialize, Serialize};
use tauri::{AppHandle, Emitter};
use tokio::sync::Mutex;
use tokio::time::sleep;
use tokio_tungstenite::{connect_async, tungstenite::Message};

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

/// Connection status emitted to the frontend
#[derive(Debug, Clone, Serialize, Deserialize)]
pub enum ConnectionStatus {
    #[serde(rename = "disconnected")]
    Disconnected,
    #[serde(rename = "connecting")]
    Connecting,
    #[serde(rename = "connected")]
    Connected,
    #[serde(rename = "reconnecting")]
    Reconnecting,
}

/// A database change event emitted to the frontend
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct CloudChangeEvent {
    pub table: String,
    pub change_type: String, // INSERT, UPDATE, DELETE
    pub record: serde_json::Value,
    pub old_record: Option<serde_json::Value>,
}

/// Phoenix Channel message (Supabase Realtime v2 protocol)
#[derive(Debug, Serialize, Deserialize)]
struct PhxMessage {
    topic: String,
    event: String,
    payload: serde_json::Value,
    #[serde(rename = "ref")]
    msg_ref: Option<String>,
}

/// Internal state shared between the connection task and commands
struct RealtimeState {
    running: bool,
    cancel_token: Option<tokio::sync::watch::Sender<bool>>,
}

/// Tauri-managed state wrapper
pub struct RealtimeManager(Arc<Mutex<RealtimeState>>);

impl Default for RealtimeManager {
    fn default() -> Self {
        Self(Arc::new(Mutex::new(RealtimeState {
            running: false,
            cancel_token: None,
        })))
    }
}

// ---------------------------------------------------------------------------
// WebSocket connection logic
// ---------------------------------------------------------------------------

/// Tables we subscribe to for real-time changes
const SUBSCRIBED_TABLES: &[&str] = &[
    "c4_tasks",
    "c4_state",
    "c4_checkpoints",
    "c1_messages",
    "c1_channels",
];

/// Build the Supabase Realtime WebSocket URL
fn build_ws_url(supabase_url: &str, api_key: &str) -> String {
    // Convert https://xyz.supabase.co → wss://xyz.supabase.co/realtime/v1/websocket
    let ws_base = supabase_url
        .replace("https://", "wss://")
        .replace("http://", "ws://");
    format!(
        "{}/realtime/v1/websocket?apikey={}&vsn=1.0.0",
        ws_base.trim_end_matches('/'),
        api_key
    )
}

/// Create a Phoenix Channel join message for a table
fn make_join_message(table: &str, ref_id: usize) -> String {
    let msg = PhxMessage {
        topic: format!("realtime:public:{}", table),
        event: "phx_join".to_string(),
        payload: serde_json::json!({
            "config": {
                "broadcast": { "self": false },
                "presence": { "key": "" },
                "postgres_changes": [{
                    "event": "*",
                    "schema": "public",
                    "table": table
                }]
            }
        }),
        msg_ref: Some(ref_id.to_string()),
    };
    serde_json::to_string(&msg).unwrap_or_default()
}

/// Create a heartbeat message
fn make_heartbeat() -> String {
    let msg = PhxMessage {
        topic: "phoenix".to_string(),
        event: "heartbeat".to_string(),
        payload: serde_json::json!({}),
        msg_ref: Some("hb".to_string()),
    };
    serde_json::to_string(&msg).unwrap_or_default()
}

/// Run the WebSocket connection with auto-reconnect
async fn run_realtime_loop(
    app: AppHandle,
    supabase_url: String,
    api_key: String,
    auth_token: String,
    mut cancel_rx: tokio::sync::watch::Receiver<bool>,
) {
    let mut backoff_secs: u64 = 1;
    let max_backoff: u64 = 30;

    loop {
        // Check cancellation
        if *cancel_rx.borrow() {
            emit_status(&app, ConnectionStatus::Disconnected);
            return;
        }

        emit_status(&app, ConnectionStatus::Connecting);

        let ws_url = build_ws_url(&supabase_url, &api_key);
        let connect_result = connect_async(&ws_url).await;

        match connect_result {
            Ok((ws_stream, _)) => {
                backoff_secs = 1; // Reset backoff on successful connection
                emit_status(&app, ConnectionStatus::Connected);

                let (mut write, mut read) = ws_stream.split();

                // Join channels for each table
                for (i, table) in SUBSCRIBED_TABLES.iter().enumerate() {
                    let join_msg = make_join_message(table, i + 1);
                    if write.send(Message::Text(join_msg.into())).await.is_err() {
                        break;
                    }
                }

                // If we have an auth token, set it via access_token event
                if !auth_token.is_empty() {
                    let token_msg = serde_json::json!({
                        "topic": "realtime:*",
                        "event": "access_token",
                        "payload": { "access_token": auth_token },
                        "ref": "auth"
                    });
                    let _ = write
                        .send(Message::Text(token_msg.to_string().into()))
                        .await;
                }

                // Heartbeat interval
                let mut heartbeat_interval = tokio::time::interval(Duration::from_secs(25));

                loop {
                    tokio::select! {
                        _ = cancel_rx.changed() => {
                            if *cancel_rx.borrow() {
                                let _ = write.send(Message::Close(None)).await;
                                emit_status(&app, ConnectionStatus::Disconnected);
                                return;
                            }
                        }
                        _ = heartbeat_interval.tick() => {
                            if write.send(Message::Text(make_heartbeat().into())).await.is_err() {
                                break; // Connection lost
                            }
                        }
                        msg = read.next() => {
                            match msg {
                                Some(Ok(Message::Text(text))) => {
                                    handle_message(&app, &text);
                                }
                                Some(Ok(Message::Close(_))) | None => {
                                    break; // Connection closed
                                }
                                Some(Err(_)) => {
                                    break; // Connection error
                                }
                                _ => {} // Ping/Pong/Binary — ignore
                            }
                        }
                    }
                }

                // Connection dropped — try to reconnect
                emit_status(&app, ConnectionStatus::Reconnecting);
            }
            Err(e) => {
                eprintln!("[realtime] connection failed: {}", e);
                emit_status(&app, ConnectionStatus::Reconnecting);
            }
        }

        // Check cancellation before sleeping
        if *cancel_rx.borrow() {
            emit_status(&app, ConnectionStatus::Disconnected);
            return;
        }

        // Exponential backoff
        sleep(Duration::from_secs(backoff_secs)).await;
        backoff_secs = (backoff_secs * 2).min(max_backoff);
    }
}

/// Parse and emit a Supabase Realtime message
fn handle_message(app: &AppHandle, text: &str) {
    let msg: serde_json::Value = match serde_json::from_str(text) {
        Ok(v) => v,
        Err(_) => return,
    };

    let event = msg.get("event").and_then(|v| v.as_str()).unwrap_or("");

    // We care about postgres_changes events
    if event != "postgres_changes" {
        return;
    }

    let topic = msg.get("topic").and_then(|v| v.as_str()).unwrap_or("");
    let table = topic
        .strip_prefix("realtime:public:")
        .unwrap_or("unknown");

    let payload = msg.get("payload").cloned().unwrap_or(serde_json::json!({}));
    let data = payload.get("data").cloned().unwrap_or(serde_json::json!({}));

    let change_type = data
        .get("type")
        .and_then(|v| v.as_str())
        .unwrap_or("UNKNOWN")
        .to_string();

    let record = data.get("record").cloned().unwrap_or(serde_json::json!({}));
    let old_record = data.get("old_record").cloned();

    let event = CloudChangeEvent {
        table: table.to_string(),
        change_type,
        record,
        old_record,
    };

    let _ = app.emit("cloud-update", &event);
}

/// Emit connection status to the frontend
fn emit_status(app: &AppHandle, status: ConnectionStatus) {
    let _ = app.emit("realtime-status", &status);
}

// ---------------------------------------------------------------------------
// Tauri IPC commands
// ---------------------------------------------------------------------------

/// Start the Supabase Realtime WebSocket connection.
///
/// Reads Supabase config from environment/files, then spawns a background
/// task that maintains the WebSocket connection with auto-reconnect.
#[tauri::command]
pub async fn realtime_connect(
    app: AppHandle,
    state: tauri::State<'_, RealtimeManager>,
) -> Result<(), String> {
    let mut inner = state.0.lock().await;

    if inner.running {
        return Ok(()); // Already connected
    }

    // Read config
    let (supabase_url, api_key) = read_config()?;
    let auth_token = read_token().unwrap_or_default();

    // Create cancellation channel
    let (cancel_tx, cancel_rx) = tokio::sync::watch::channel(false);
    inner.cancel_token = Some(cancel_tx);
    inner.running = true;
    drop(inner);

    // Spawn the WebSocket loop
    tokio::spawn(async move {
        run_realtime_loop(app, supabase_url, api_key, auth_token, cancel_rx).await;
    });

    Ok(())
}

/// Disconnect the Supabase Realtime WebSocket.
#[tauri::command]
pub async fn realtime_disconnect(
    state: tauri::State<'_, RealtimeManager>,
) -> Result<(), String> {
    let mut inner = state.0.lock().await;
    if let Some(tx) = inner.cancel_token.take() {
        let _ = tx.send(true);
    }
    inner.running = false;
    Ok(())
}

/// Check if the realtime connection is active.
#[tauri::command]
pub async fn realtime_status(
    state: tauri::State<'_, RealtimeManager>,
) -> Result<bool, String> {
    let inner = state.0.lock().await;
    Ok(inner.running)
}

// ---------------------------------------------------------------------------
// Config helpers (reuse patterns from cloud.rs)
// ---------------------------------------------------------------------------

fn read_config() -> Result<(String, String), String> {
    // Try env vars first
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
        return Err("Supabase not configured".to_string());
    }
    let content = std::fs::read_to_string(&config_path)
        .map_err(|e| format!("Failed to read supabase config: {}", e))?;
    let config: serde_json::Value = serde_json::from_str(&content)
        .map_err(|e| format!("Invalid supabase config: {}", e))?;
    let url = config
        .get("url")
        .and_then(|v| v.as_str())
        .ok_or("Missing 'url' in supabase config")?;
    let key = config
        .get("anon_key")
        .and_then(|v| v.as_str())
        .ok_or("Missing 'anon_key' in supabase config")?;
    Ok((url.to_string(), key.to_string()))
}

fn read_token() -> Result<String, String> {
    let home = dirs::home_dir().ok_or("Could not find home directory")?;
    let path = home.join(".c4").join("session.json");
    if !path.exists() {
        return Err("Not logged in".to_string());
    }
    let content =
        std::fs::read_to_string(&path).map_err(|e| format!("Failed to read session: {}", e))?;
    let session: serde_json::Value =
        serde_json::from_str(&content).map_err(|e| format!("Invalid session: {}", e))?;
    session
        .get("access_token")
        .and_then(|v| v.as_str())
        .map(String::from)
        .ok_or("No access_token in session".to_string())
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_build_ws_url() {
        let url = build_ws_url("https://abc.supabase.co", "my-key");
        assert_eq!(
            url,
            "wss://abc.supabase.co/realtime/v1/websocket?apikey=my-key&vsn=1.0.0"
        );
    }

    #[test]
    fn test_build_ws_url_trailing_slash() {
        let url = build_ws_url("https://abc.supabase.co/", "key");
        assert!(url.starts_with("wss://abc.supabase.co/realtime/"));
    }

    #[test]
    fn test_make_join_message_parses() {
        let msg = make_join_message("c4_tasks", 1);
        let parsed: serde_json::Value = serde_json::from_str(&msg).unwrap();
        assert_eq!(parsed["topic"], "realtime:public:c4_tasks");
        assert_eq!(parsed["event"], "phx_join");
        assert_eq!(parsed["ref"], "1");
        let changes = &parsed["payload"]["config"]["postgres_changes"];
        assert_eq!(changes[0]["table"], "c4_tasks");
    }

    #[test]
    fn test_make_heartbeat_parses() {
        let msg = make_heartbeat();
        let parsed: serde_json::Value = serde_json::from_str(&msg).unwrap();
        assert_eq!(parsed["topic"], "phoenix");
        assert_eq!(parsed["event"], "heartbeat");
    }

    #[test]
    fn test_cloud_change_event_serialization() {
        let event = CloudChangeEvent {
            table: "c4_tasks".to_string(),
            change_type: "INSERT".to_string(),
            record: serde_json::json!({"task_id": "T-001-0"}),
            old_record: None,
        };
        let json = serde_json::to_string(&event).unwrap();
        assert!(json.contains("c4_tasks"));
        assert!(json.contains("INSERT"));
        let parsed: CloudChangeEvent = serde_json::from_str(&json).unwrap();
        assert_eq!(parsed.table, "c4_tasks");
    }

    #[test]
    fn test_connection_status_serialization() {
        let status = ConnectionStatus::Connected;
        let json = serde_json::to_string(&status).unwrap();
        assert_eq!(json, "\"connected\"");
    }
}
