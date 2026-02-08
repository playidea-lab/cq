//! File system watcher for session directory auto-refresh
//!
//! Watches the Claude Code sessions directory and emits Tauri events
//! when new session files are created or modified.

use std::sync::mpsc;
use std::time::Duration;

use notify::{RecommendedWatcher, RecursiveMode, Watcher};
use tauri::{AppHandle, Emitter};

/// Event payload sent to the frontend
#[derive(Clone, serde::Serialize)]
pub struct SessionChangeEvent {
    pub kind: String, // "created", "modified", "removed"
    pub path: String,
}

/// Start watching the Claude Code sessions directory for a project.
/// Emits "sessions-changed" events to the frontend when files change.
pub fn start_session_watcher(app: &AppHandle, project_path: &str) -> Result<(), String> {
    let home = dirs::home_dir().ok_or("Could not find home directory")?;

    // Claude Code sessions dir
    let slug = project_path.replace('/', "-").replace('\\', "-");
    let slug = if slug.starts_with('-') { slug } else { format!("-{}", slug) };
    let sessions_dir = home.join(".claude").join("projects").join(&slug);

    if !sessions_dir.exists() {
        return Ok(()); // No sessions dir yet, nothing to watch
    }

    let app_handle = app.clone();
    let sessions_dir_str = sessions_dir.to_string_lossy().to_string();
    let (tx, rx) = mpsc::channel();

    // Create watcher with debounce
    let mut watcher: RecommendedWatcher = Watcher::new(
        tx,
        notify::Config::default().with_poll_interval(Duration::from_secs(2)),
    )
    .map_err(|e| format!("Failed to create watcher: {}", e))?;

    watcher
        .watch(&sessions_dir, RecursiveMode::NonRecursive)
        .map_err(|e| format!("Failed to watch directory: {}", e))?;

    // Spawn a thread to process events and emit to frontend
    std::thread::spawn(move || {
        // Keep watcher alive in this thread
        let _watcher = watcher;

        // Debounce: only emit once per 2 seconds
        let mut last_emit = std::time::Instant::now()
            .checked_sub(Duration::from_secs(5))
            .unwrap_or_else(std::time::Instant::now);

        loop {
            match rx.recv_timeout(Duration::from_secs(5)) {
                Ok(Ok(event)) => {
                    let now = std::time::Instant::now();
                    if now.duration_since(last_emit) < Duration::from_secs(2) {
                        continue; // Debounce
                    }

                    let kind = match event.kind {
                        notify::EventKind::Create(_) => "created",
                        notify::EventKind::Modify(_) => "modified",
                        notify::EventKind::Remove(_) => "removed",
                        _ => continue,
                    };

                    let path = event
                        .paths
                        .first()
                        .map(|p| p.to_string_lossy().to_string())
                        .unwrap_or_default();

                    // Only care about .jsonl files
                    if !path.ends_with(".jsonl") {
                        continue;
                    }

                    // Invalidate session cache for this project
                    crate::commands::invalidate_session_cache(&sessions_dir_str);

                    let _ = app_handle.emit(
                        "sessions-changed",
                        SessionChangeEvent {
                            kind: kind.to_string(),
                            path,
                        },
                    );
                    last_emit = now;
                }
                Ok(Err(_)) => continue,
                Err(mpsc::RecvTimeoutError::Timeout) => continue,
                Err(mpsc::RecvTimeoutError::Disconnected) => break,
            }
        }
    });

    Ok(())
}
