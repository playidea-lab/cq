//! C1 — Unified Dashboard Messenger
//!
//! Desktop app for team collaboration where users and agents participate
//! as equal members. Integrates messaging, documents, and project management.

pub mod analytics;
pub mod auth;
pub mod cloud;
pub mod commands;
pub mod documents;
pub mod eventbus;
pub mod knowledge;
pub mod layout;
pub mod messaging;
pub mod models;
pub mod providers;
pub mod realtime;
pub mod scanner;
pub mod watcher;

use tauri::Manager;

#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
    // Load .env from project root (two levels up from src-tauri)
    // Try multiple locations: CWD, parent dirs, ~/.c4/.env
    for candidate in [".env", "../.env", "../../.env"] {
        if dotenvy::from_filename(candidate).is_ok() {
            break;
        }
    }
    if let Some(home) = dirs::home_dir() {
        let _ = dotenvy::from_path(home.join(".c4").join(".env"));
    }

    // Panic hook: write crash info to file
    std::panic::set_hook(Box::new(|info| {
        let msg = format!("[PANIC] {}\n{:?}", info, std::backtrace::Backtrace::capture());
        eprintln!("{}", msg);
        let _ = std::fs::write("/tmp/c1-crash.log", &msg);
    }));

    tauri::Builder::default()
        .plugin(tauri_plugin_dialog::init())
        .plugin(tauri_plugin_shell::init())
        .manage(realtime::RealtimeManager::default())
        .manage(eventbus::EventBusManager::default())
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
            commands::write_config_file,
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
            // Session analytics
            analytics::get_session_stats,
            analytics::get_provider_timeline,
            // Dashboard enhancement (Phase 2)
            commands::get_task_timeline,
            commands::get_worker_activity,
            commands::get_validation_results,
            // Project ID resolution
            commands::get_project_id_cmd,
            // Git Graph
            commands::get_git_graph,
            // Cloud sync
            cloud::cloud_sync_tasks,
            cloud::cloud_get_team_projects,
            cloud::cloud_get_remote_dashboard,
            cloud::cloud_pull_tasks,
            cloud::cloud_sync_status,
            cloud::cloud_get_checkpoints,
            cloud::cloud_get_growth_metrics,
            cloud::cloud_get_agent_traces,
            // Knowledge (Phase 8.3)
            cloud::cloud_get_knowledge_docs,
            cloud::cloud_search_knowledge,
            // Realtime
            realtime::realtime_connect,
            realtime::realtime_disconnect,
            realtime::realtime_status,
            // EventBus
            eventbus::eventbus_connect,
            eventbus::eventbus_disconnect,
            eventbus::eventbus_status,
            // Auth
            auth::auth_get_session,
            auth::auth_login,
            auth::auth_logout,
            auth::auth_refresh,
            auth::auth_get_config,
            // Messaging
            messaging::list_channels,
            messaging::get_channel_messages,
            messaging::send_message,
            messaging::search_messages,
            messaging::mark_read,
            messaging::create_channel,
            messaging::get_channel_summary,
            // Members
            messaging::list_members,
            messaging::update_presence,
            // Channel Pins
            messaging::create_channel_pin,
            messaging::list_channel_pins,
            messaging::delete_channel_pin,
            // Documents
            documents::list_documents,
            documents::get_document,
            documents::save_document,
            documents::create_document,
            documents::delete_document,
            // Knowledge
            knowledge::list_knowledge,
            knowledge::get_knowledge_doc,
            knowledge::get_knowledge_stats,
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
