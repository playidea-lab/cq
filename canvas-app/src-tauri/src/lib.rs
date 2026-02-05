//! C4 Canvas - Auto-Map Project Visualizer
//!
//! This library provides the backend functionality for scanning C4 projects
//! and generating canvas visualizations.

pub mod commands;
pub mod layout;
pub mod models;
pub mod scanner;

use tauri::Manager;

#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
    tauri::Builder::default()
        .plugin(tauri_plugin_dialog::init())
        .plugin(tauri_plugin_shell::init())
        .invoke_handler(tauri::generate_handler![
            commands::scan_project_cmd,
            commands::save_canvas,
            commands::load_canvas,
        ])
        .setup(|app| {
            // Log startup
            #[cfg(debug_assertions)]
            {
                let window = app.get_webview_window("main").unwrap();
                window.open_devtools();
            }
            Ok(())
        })
        .run(tauri::generate_context!())
        .expect("error while running tauri application");
}
