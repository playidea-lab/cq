//! Hands — Execution Bridge for AI Agents
//!
//! Provides a WebSocket server for c5 (Go) to send native commands
//! to the C1 desktop host.

use std::sync::Arc;
use std::path::PathBuf;
use std::io::Cursor;
use serde::{Deserialize, Serialize};
use tauri::{AppHandle, Emitter};
use tokio::net::{TcpListener, TcpStream};
use tokio_tungstenite::accept_async;
use futures_util::{StreamExt, SinkExt};
use base64::{Engine as _, engine::general_purpose};
use screenshots::Screen;

#[derive(Debug, Serialize, Deserialize, Clone)]
struct HandsOutputPayload {
    message: String,
}

#[derive(Debug, Serialize, Deserialize)]
pub struct HandsRequest {
    pub method: String,
    pub params: serde_json::Value,
    pub id: Option<u64>,
}

#[derive(Debug, Serialize, Deserialize)]
pub struct HandsResponse {
    pub result: Option<serde_json::Value>,
    pub error: Option<String>,
    pub id: Option<u64>,
}

pub struct HandsManager {
    pub port: u16,
}

impl HandsManager {
    pub fn new(port: u16) -> Self {
        Self { port }
    }

    pub async fn start_server(self: Arc<Self>, app_handle: AppHandle) {
        let addr = format!("127.0.0.1:{}", self.port);
        let listener = match TcpListener::bind(&addr).await {
            Ok(l) => l,
            Err(e) => {
                eprintln!("[Hands] Failed to bind to {}: {}", addr, e);
                return;
            }
        };

        println!("[Hands] WebSocket server listening on ws://{}", addr);

        while let Ok((stream, _)) = listener.accept().await {
            let manager = Arc::clone(&self);
            let app = app_handle.clone();
            tokio::spawn(async move {
                manager.handle_connection(stream, app).await;
            });
        }
    }

    async fn handle_connection(&self, stream: TcpStream, app: AppHandle) {
        let ws_stream = match accept_async(stream).await {
            Ok(s) => s,
            Err(e) => {
                eprintln!("[Hands] Error during websocket handshake: {}", e);
                return;
            }
        };

        let (mut ws_sender, mut ws_receiver) = ws_stream.split();

        while let Some(msg) = ws_receiver.next().await {
            match msg {
                Ok(msg) if msg.is_text() || msg.is_binary() => {
                    let text = msg.to_text().unwrap_or("");
                    let response = self.process_request(text, &app).await;
                    let resp_json = serde_json::to_string(&response).unwrap_or_default();
                    if let Err(e) = ws_sender.send(tokio_tungstenite::tungstenite::Message::Text(resp_json.into())).await {
                        eprintln!("[Hands] Error sending response: {}", e);
                        break;
                    }
                }
                Ok(msg) if msg.is_close() => break,
                Err(e) => {
                    eprintln!("[Hands] Websocket error: {}", e);
                    break;
                }
                _ => {}
            }
        }
    }

    async fn process_request(&self, text: &str, app: &AppHandle) -> HandsResponse {
        let req: HandsRequest = match serde_json::from_str(text) {
            Ok(r) => r,
            Err(e) => return HandsResponse { result: None, error: Some(format!("Invalid JSON: {}", e)), id: None },
        };

        let id = req.id;
        match req.method.as_str() {
            "Ping" => HandsResponse { result: Some(serde_json::json!({"status": "pong"})), error: None, id },
            "ExecuteShell" => self.handle_execute_shell(req.params, id, app).await,
            "CaptureScreenshot" => self.handle_capture_screenshot(id).await,
            _ => HandsResponse { result: None, error: Some(format!("Unknown method: {}", req.method)), id },
        }
    }

    async fn handle_execute_shell(&self, params: serde_json::Value, id: Option<u64>, app: &AppHandle) -> HandsResponse {
        let command = params.get("command").and_then(|v| v.as_str()).unwrap_or("");
        let args: Vec<&str> = params.get("args")
            .and_then(|v| v.as_array())
            .map(|a| a.iter().filter_map(|v| v.as_str()).collect())
            .unwrap_or_default();

        if command.is_empty() {
            return HandsResponse { result: None, error: Some("Command is required".to_string()), id };
        }

        if !self.is_command_allowed(command) {
            return HandsResponse { result: None, error: Some(format!("Command not allowed: {}", command)), id };
        }

        let cwd = std::env::current_dir().unwrap_or_else(|_| PathBuf::from("."));

        let _ = app.emit("hands-output", HandsOutputPayload { 
            message: format!("\r\n\x1b[1;34m[Executing]\x1b[0m {} {:?}\r\n", command, args) 
        });

        match tokio::process::Command::new(command)
            .args(&args)
            .current_dir(&cwd)
            .output()
            .await 
        {
            Ok(output) => {
                let stdout = String::from_utf8_lossy(&output.stdout).to_string();
                let stderr = String::from_utf8_lossy(&output.stderr).to_string();
                let exit_code = output.status.code();

                if !stdout.is_empty() {
                    let _ = app.emit("hands-output", HandsOutputPayload { message: stdout.clone() });
                }
                if !stderr.is_empty() {
                    let _ = app.emit("hands-output", HandsOutputPayload { message: format!("\x1b[31m{}\x1b[0m", stderr) });
                }

                HandsResponse {
                    result: Some(serde_json::json!({
                        "stdout": self.truncate_output(stdout),
                        "stderr": self.truncate_output(stderr),
                        "exit_code": exit_code
                    })),
                    error: None,
                    id
                }
            }
            Err(e) => {
                let err_msg = format!("Execution failed: {}", e);
                let _ = app.emit("hands-output", HandsOutputPayload { message: format!("\x1b[31m{}\x1b[0m\r\n", err_msg) });
                HandsResponse { result: None, error: Some(err_msg), id }
            },
        }
    }

    async fn handle_capture_screenshot(&self, id: Option<u64>) -> HandsResponse {
        let result = tokio::task::spawn_blocking(move || {
            let screens = Screen::all().map_err(|e| e.to_string())?;
            if let Some(screen) = screens.first() {
                let image = screen.capture().map_err(|e| e.to_string())?;
                let mut buffer = Vec::new();
                let mut cursor = Cursor::new(&mut buffer);
                image.write_to(&mut cursor, screenshots::image::ImageFormat::Png)
                    .map_err(|e| e.to_string())?;
                let b64 = general_purpose::STANDARD.encode(buffer);
                Ok(b64)
            } else {
                Err("No screens found".to_string())
            }
        }).await;

        match result {
            Ok(Ok(b64)) => HandsResponse {
                result: Some(serde_json::json!({ "image_b64": b64, "format": "png" })),
                error: None,
                id
            },
            Ok(Err(e)) => HandsResponse { result: None, error: Some(e), id },
            Err(e) => HandsResponse { result: None, error: Some(e.to_string()), id },
        }
    }

    fn is_command_allowed(&self, _command: &str) -> bool {
        true
    }

    fn truncate_output(&self, input: String) -> String {
        let max_len = 100_000;
        if input.len() > max_len {
            format!("{}... [Truncated]", &input[..max_len])
        } else {
            input
        }
    }
}
