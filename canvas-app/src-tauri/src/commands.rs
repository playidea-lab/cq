//! Tauri IPC commands

use std::fs;
use std::path::Path;

use crate::models::{CanvasData, ScanResult};
use crate::scanner::scan_project;

/// Canvas save file name
const CANVAS_FILE: &str = ".c4/canvas.json";

/// Scan a project directory and return canvas data
#[tauri::command(rename_all = "snake_case")]
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
