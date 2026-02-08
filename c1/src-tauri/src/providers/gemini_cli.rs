//! Gemini CLI session provider (stub)
//!
//! Detects `~/.gemini/` config directory. Gemini CLI does not currently
//! store session data in a readable format, so this provider only reports
//! its presence and returns empty session lists.

use std::path::PathBuf;

use crate::models::{SessionMeta, SessionPage};
use super::{ProviderInfo, ProviderKind, SessionProvider};

pub struct GeminiCliProvider;

fn gemini_dir() -> Result<PathBuf, String> {
    let home = dirs::home_dir().ok_or("Could not find home directory")?;
    Ok(home.join(".gemini"))
}

impl SessionProvider for GeminiCliProvider {
    fn info(&self, _project_path: &str) -> Result<ProviderInfo, String> {
        let dir = gemini_dir()?;
        if !dir.exists() {
            return Err("Gemini CLI not installed".to_string());
        }

        Ok(ProviderInfo {
            kind: ProviderKind::GeminiCli,
            name: "Gemini CLI".to_string(),
            icon: "G".to_string(),
            session_count: 0,
            data_path: dir.to_string_lossy().to_string(),
            is_global: true,
        })
    }

    fn list_sessions(&self, _project_path: &str) -> Result<Vec<SessionMeta>, String> {
        Ok(Vec::new())
    }

    fn get_messages(
        &self,
        _session_id: &str,
        _offset: u32,
        _limit: u32,
    ) -> Result<SessionPage, String> {
        Ok(SessionPage {
            messages: Vec::new(),
            total_lines: 0,
            has_more: false,
        })
    }
}
