//! Test Hands Server with Screenshot and Shell Execution support
//! Run with: cd c1/src-tauri && cargo run --bin test_hands_server

use tokio::net::TcpListener;
use tokio_tungstenite::accept_async;
use futures_util::{StreamExt, SinkExt};
use serde::{Deserialize, Serialize};
use std::path::PathBuf;
use std::io::Cursor;
use base64::{Engine as _, engine::general_purpose};
use screenshots::Screen;

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

async fn handle_connection(stream: tokio::net::TcpStream) {
    let ws_stream = accept_async(stream).await.expect("Error during websocket handshake");
    let (mut ws_sender, mut ws_receiver) = ws_stream.split();

    while let Some(msg) = ws_receiver.next().await {
        match msg {
            Ok(msg) if msg.is_text() => {
                let text = msg.to_text().unwrap_or("");
                let req: HandsRequest = serde_json::from_str(text).unwrap();
                let id = req.id;

                let response = match req.method.as_str() {
                    "Ping" => HandsResponse { result: Some(serde_json::json!({"status": "pong"})), error: None, id },
                    "ExecuteShell" => {
                        let command = req.params.get("command").and_then(|v| v.as_str()).unwrap_or("");
                        let args: Vec<&str> = req.params.get("args")
                            .and_then(|v| v.as_array())
                            .map(|a| a.iter().filter_map(|v| v.as_str()).collect())
                            .unwrap_or_default();

                        println!("[TestServer] Executing: {} {:?}", command, args);
                        let cwd = std::env::current_dir().unwrap_or_else(|_| PathBuf::from("."));

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
                                HandsResponse {
                                    result: Some(serde_json::json!({
                                        "stdout": stdout,
                                        "stderr": stderr,
                                        "exit_code": exit_code
                                    })),
                                    error: None,
                                    id
                                }
                            }
                            Err(e) => HandsResponse { result: None, error: Some(format!("Execution failed: {}", e)), id },
                        }
                    },
                    "CaptureScreenshot" => {
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
                    },
                    _ => HandsResponse { result: None, error: Some("Unknown method".to_string()), id },
                };

                let resp_json = serde_json::to_string(&response).unwrap();
                ws_sender.send(tokio_tungstenite::tungstenite::Message::Text(resp_json.into())).await.unwrap();
            }
            _ => break,
        }
    }
}

#[tokio::main]
async fn main() {
    let addr = "127.0.0.1:8586";
    let listener = TcpListener::bind(&addr).await.expect("Can't bind");
    println!("[TestServer] Listening on: ws://{}", addr);

    while let Ok((stream, _)) = listener.accept().await {
        tokio::spawn(handle_connection(stream));
    }
}
