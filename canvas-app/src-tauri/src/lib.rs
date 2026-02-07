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
    // Panic hook: write crash info to file
    std::panic::set_hook(Box::new(|info| {
        let msg = format!("[PANIC] {}\n{:?}", info, std::backtrace::Backtrace::capture());
        eprintln!("{}", msg);
        let _ = std::fs::write("/tmp/c4-canvas-crash.log", &msg);
    }));

    tauri::Builder::default()
        .plugin(tauri_plugin_dialog::init())
        .plugin(tauri_plugin_shell::init())
        .invoke_handler(tauri::generate_handler![
            commands::scan_project_cmd,
            commands::save_canvas,
            commands::load_canvas,
            commands::get_project_state,
            commands::get_tasks,
            commands::get_task_detail,
            commands::list_sessions,
            commands::get_session_messages,
            commands::get_session_file_changes,
            commands::list_config_files,
            commands::read_config_file,
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
