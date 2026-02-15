//! EventBus WebSocket client
//!
//! Connects to the C3 EventBus WebSocket bridge (Go server) and emits
//! Tauri events when EventBus events occur.
//!
//! Usage: call `eventbus_connect` from the frontend to start the connection,
//! then listen for "eventbus-event" events. Call `eventbus_disconnect` to stop.

use std::sync::Arc;
use std::time::Duration;

use futures_util::StreamExt;
use serde::{Deserialize, Serialize};
use tauri::{AppHandle, Emitter};
use tokio::sync::Mutex;
use tokio::time::sleep;
use tokio_tungstenite::{connect_async, tungstenite::Message};

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

/// EventBus event emitted to the frontend
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct EventBusEvent {
    pub id: String,
    #[serde(rename = "type")]
    pub event_type: String,
    pub source: String,
    pub data: serde_json::Value,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub project_id: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub correlation_id: Option<String>,
    pub timestamp_ms: i64,
}

/// Connection status emitted to the frontend
#[derive(Debug, Clone, Serialize, Deserialize)]
pub enum EventBusStatus {
    #[serde(rename = "connected")]
    Connected,
    #[serde(rename = "disconnected")]
    Disconnected,
    #[serde(rename = "reconnecting")]
    Reconnecting,
}

/// Internal state
struct EventBusState {
    running: bool,
    cancel_token: Option<tokio::sync::watch::Sender<bool>>,
}

/// Tauri-managed state wrapper
pub struct EventBusManager(Arc<Mutex<EventBusState>>);

impl Default for EventBusManager {
    fn default() -> Self {
        Self(Arc::new(Mutex::new(EventBusState {
            running: false,
            cancel_token: None,
        })))
    }
}

// ---------------------------------------------------------------------------
// WebSocket connection
// ---------------------------------------------------------------------------

const DEFAULT_WS_URL: &str = "ws://127.0.0.1:7124/ws/events?pattern=*";

/// Run the WebSocket connection with auto-reconnect
async fn run_eventbus_loop(
    app: AppHandle,
    ws_url: String,
    mut cancel_rx: tokio::sync::watch::Receiver<bool>,
) {
    let mut backoff_secs: u64 = 1;
    let max_backoff: u64 = 30;

    loop {
        if *cancel_rx.borrow() {
            emit_status(&app, EventBusStatus::Disconnected);
            return;
        }

        let connect_result = connect_async(&ws_url).await;

        match connect_result {
            Ok((ws_stream, _)) => {
                backoff_secs = 1;
                emit_status(&app, EventBusStatus::Connected);

                let (_write, mut read) = ws_stream.split();

                loop {
                    tokio::select! {
                        _ = cancel_rx.changed() => {
                            if *cancel_rx.borrow() {
                                emit_status(&app, EventBusStatus::Disconnected);
                                return;
                            }
                        }
                        msg = read.next() => {
                            match msg {
                                Some(Ok(Message::Text(text))) => {
                                    if let Ok(event) = serde_json::from_str::<EventBusEvent>(&text) {
                                        let _ = app.emit("eventbus-event", &event);
                                    }
                                }
                                Some(Ok(Message::Close(_))) | None => {
                                    break;
                                }
                                Some(Err(_)) => {
                                    break;
                                }
                                _ => {}
                            }
                        }
                    }
                }

                // Status will be emitted after cancel check below
            }
            Err(e) => {
                eprintln!("[eventbus] connection failed: {}", e);
            }
        }

        if *cancel_rx.borrow() {
            emit_status(&app, EventBusStatus::Disconnected);
            return;
        }

        emit_status(&app, EventBusStatus::Reconnecting);
        sleep(Duration::from_secs(backoff_secs)).await;
        backoff_secs = (backoff_secs * 2).min(max_backoff);
    }
}

fn emit_status(app: &AppHandle, status: EventBusStatus) {
    let _ = app.emit("eventbus-status", &status);
}

/// Resolve the WebSocket URL from environment or config
fn resolve_ws_url() -> String {
    // Check env first
    if let Ok(port) = std::env::var("C4_EVENTBUS_WS_PORT") {
        return format!("ws://127.0.0.1:{}/ws/events?pattern=*", port);
    }

    // Check ~/.c4/config.yaml for eventbus.ws_port
    if let Some(home) = dirs::home_dir() {
        let config_path = home.join(".c4").join("config.yaml");
        if let Ok(content) = std::fs::read_to_string(&config_path) {
            if let Ok(yaml) = serde_yaml::from_str::<serde_json::Value>(&content) {
                if let Some(port) = yaml
                    .get("eventbus")
                    .and_then(|eb| eb.get("ws_port"))
                    .and_then(|p| p.as_u64())
                {
                    if port > 0 {
                        return format!("ws://127.0.0.1:{}/ws/events?pattern=*", port);
                    }
                }
            }
        }
    }

    DEFAULT_WS_URL.to_string()
}

// ---------------------------------------------------------------------------
// Tauri IPC commands
// ---------------------------------------------------------------------------

/// Start the EventBus WebSocket connection.
#[tauri::command]
pub async fn eventbus_connect(
    app: AppHandle,
    state: tauri::State<'_, EventBusManager>,
) -> Result<(), String> {
    let mut inner = state.0.lock().await;

    if inner.running {
        return Ok(());
    }

    let ws_url = resolve_ws_url();

    let (cancel_tx, cancel_rx) = tokio::sync::watch::channel(false);
    inner.cancel_token = Some(cancel_tx);
    inner.running = true;
    drop(inner);

    tokio::spawn(async move {
        run_eventbus_loop(app, ws_url, cancel_rx).await;
    });

    Ok(())
}

/// Disconnect the EventBus WebSocket.
#[tauri::command]
pub async fn eventbus_disconnect(
    state: tauri::State<'_, EventBusManager>,
) -> Result<(), String> {
    let mut inner = state.0.lock().await;
    if let Some(tx) = inner.cancel_token.take() {
        let _ = tx.send(true);
    }
    inner.running = false;
    Ok(())
}

/// Check if the EventBus connection is active.
#[tauri::command]
pub async fn eventbus_status(
    state: tauri::State<'_, EventBusManager>,
) -> Result<bool, String> {
    let inner = state.0.lock().await;
    Ok(inner.running)
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_eventbus_event_serialization() {
        let event = EventBusEvent {
            id: "ev-123".to_string(),
            event_type: "task.completed".to_string(),
            source: "c4.core".to_string(),
            data: serde_json::json!({"task_id": "T-001-0"}),
            project_id: Some("proj1".to_string()),
            correlation_id: Some("corr-abc".to_string()),
            timestamp_ms: 1700000000000,
        };
        let json = serde_json::to_string(&event).unwrap();
        assert!(json.contains("task.completed"));
        assert!(json.contains("corr-abc"));
        let parsed: EventBusEvent = serde_json::from_str(&json).unwrap();
        assert_eq!(parsed.id, "ev-123");
        assert_eq!(parsed.event_type, "task.completed");
    }

    #[test]
    fn test_eventbus_event_deserialization_from_ws() {
        // Simulate what Go WSBridge sends
        let ws_json = r#"{"id":"ev-456","type":"drive.uploaded","source":"c4.drive","data":{"path":"/test.pdf"},"timestamp_ms":1700000000000}"#;
        let event: EventBusEvent = serde_json::from_str(ws_json).unwrap();
        assert_eq!(event.id, "ev-456");
        assert_eq!(event.event_type, "drive.uploaded");
        assert!(event.correlation_id.is_none());
    }

    #[test]
    fn test_eventbus_status_serialization() {
        let status = EventBusStatus::Connected;
        let json = serde_json::to_string(&status).unwrap();
        assert_eq!(json, "\"connected\"");
    }

    #[test]
    fn test_resolve_ws_url_default() {
        // When no env/config, should return default
        let url = DEFAULT_WS_URL;
        assert!(url.contains("127.0.0.1:7124"));
    }
}
