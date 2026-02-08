//! C1 (See) — Multi-LLM Tool Explorer
//!
//! Desktop app for exploring sessions from Claude Code, Codex CLI,
//! Cursor, and other LLM coding tools.

pub mod auth;
pub mod commands;
pub mod layout;
pub mod models;
pub mod providers;
pub mod scanner;
pub mod watcher;

use tauri::Manager;

#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
    // Panic hook: write crash info to file
    std::panic::set_hook(Box::new(|info| {
        let msg = format!("[PANIC] {}\n{:?}", info, std::backtrace::Backtrace::capture());
        eprintln!("{}", msg);
        let _ = std::fs::write("/tmp/c1-crash.log", &msg);
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
            // Session content search
            commands::search_sessions,
            // Provider-based commands
            commands::list_providers,
            commands::list_sessions_for_provider,
            commands::get_provider_session_messages,
            commands::get_provider_token_usage,
            // Editor deeplink commands
            commands::detect_editors,
            commands::open_in_editor,
            // File watcher
            commands::watch_sessions,
            // Auth
            auth::auth_get_session,
            auth::auth_login,
            auth::auth_logout,
            auth::auth_refresh,
            auth::auth_get_config,
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
