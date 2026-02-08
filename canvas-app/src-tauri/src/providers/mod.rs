//! Multi-LLM session providers
//!
//! Each provider knows how to discover and parse sessions from a specific
//! LLM coding tool (Claude Code, Codex CLI, Cursor, etc.).

pub mod claude_code;
pub mod codex_cli;
pub mod cursor;
pub mod gemini_cli;

use crate::models::{SessionMeta, SessionPage};
use serde::{Deserialize, Serialize};

/// Which LLM tool a provider represents
#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub enum ProviderKind {
    ClaudeCode,
    CodexCli,
    Cursor,
    GeminiCli,
}

/// Summary information about a detected provider
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ProviderInfo {
    pub kind: ProviderKind,
    pub name: String,
    pub icon: String,
    pub session_count: usize,
    pub data_path: String,
}

/// Trait that every session provider must implement.
///
/// All methods are synchronous and expected to be called inside
/// `tokio::task::spawn_blocking`.
pub trait SessionProvider {
    /// Return metadata about this provider (name, icon, session count, etc.)
    fn info(&self, project_path: &str) -> Result<ProviderInfo, String>;

    /// List available sessions, sorted newest-first.
    fn list_sessions(&self, project_path: &str) -> Result<Vec<SessionMeta>, String>;

    /// Get paginated messages for a single session.
    fn get_messages(
        &self,
        session_id: &str,
        offset: u32,
        limit: u32,
    ) -> Result<SessionPage, String>;
}

/// Detect which providers are installed on this machine and return their info.
pub fn detect_providers(project_path: &str) -> Vec<ProviderInfo> {
    let mut providers = Vec::new();

    // Claude Code
    let cc = claude_code::ClaudeCodeProvider;
    if let Ok(info) = cc.info(project_path) {
        providers.push(info);
    }

    // Codex CLI
    let codex = codex_cli::CodexCliProvider;
    if let Ok(info) = codex.info(project_path) {
        providers.push(info);
    }

    // Cursor
    let cur = cursor::CursorProvider;
    if let Ok(info) = cur.info(project_path) {
        providers.push(info);
    }

    // Gemini CLI (stub)
    let gemini = gemini_cli::GeminiCliProvider;
    if let Ok(info) = gemini.info(project_path) {
        providers.push(info);
    }

    providers
}

/// Route a provider call by kind
pub fn get_provider(kind: ProviderKind) -> Box<dyn SessionProvider + Send> {
    match kind {
        ProviderKind::ClaudeCode => Box::new(claude_code::ClaudeCodeProvider),
        ProviderKind::CodexCli => Box::new(codex_cli::CodexCliProvider),
        ProviderKind::Cursor => Box::new(cursor::CursorProvider),
        ProviderKind::GeminiCli => Box::new(gemini_cli::GeminiCliProvider),
    }
}
