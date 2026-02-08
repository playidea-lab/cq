//! Canvas data models

use serde::{Deserialize, Serialize};
use std::collections::HashMap;

/// Type of node in the canvas
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
#[serde(rename_all = "lowercase")]
pub enum NodeType {
    Document,
    Config,
    Session,
    Task,
    Connection,
}

impl NodeType {
    pub fn from_path(path: &str) -> Self {
        let lower = path.to_lowercase();

        // Directory context classification
        if lower.contains("docs/") || lower.starts_with("docs/") {
            return Self::Document;
        }
        if lower.contains(".claude/") || lower.starts_with(".claude/") {
            return Self::Config;
        }
        if lower.contains(".c4/") || lower.starts_with(".c4/") {
            // Special handling for .c4 subdirectories
            if lower.contains(".c4/tasks") || lower.contains(".c4/events") {
                return Self::Task;
            }
            return Self::Config;
        }

        // Build configuration files
        if lower.ends_with("package.json")
            || lower.ends_with("cargo.toml")
            || lower.ends_with("pyproject.toml")
            || lower.ends_with("tsconfig.json")
            || lower.ends_with("build.gradle") {
            return Self::Config;
        }

        // Environment and connection files
        if lower.ends_with(".env")
            || lower.ends_with(".env.local")
            || lower.ends_with(".env.production")
            || lower.ends_with(".env.development") {
            return Self::Connection;
        }

        // MCP and connection configurations
        if lower.contains("mcp") || lower.contains("connection") {
            return Self::Connection;
        }

        // Session and history files
        if lower.contains("session") || lower.contains("history") || lower.ends_with(".jsonl") {
            return Self::Session;
        }

        // Task-related files
        if lower.contains("task") {
            return Self::Task;
        }

        // General configuration files
        if lower.ends_with(".yaml") || lower.ends_with(".yml") || lower.ends_with(".json") || lower.ends_with(".toml") {
            return Self::Config;
        }

        // Default to Document
        Self::Document
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_directory_context_classification() {
        // docs/ directory should be Document
        assert_eq!(NodeType::from_path("docs/README.md"), NodeType::Document);
        assert_eq!(NodeType::from_path("docs/api.yaml"), NodeType::Document);

        // .claude/ directory should be Config
        assert_eq!(NodeType::from_path(".claude/rules/test.md"), NodeType::Config);
        assert_eq!(NodeType::from_path(".claude/CLAUDE.md"), NodeType::Config);

        // .c4/ directory subdivisions
        assert_eq!(NodeType::from_path(".c4/config.yaml"), NodeType::Config);
        assert_eq!(NodeType::from_path(".c4/tasks.db"), NodeType::Task);
        assert_eq!(NodeType::from_path(".c4/events/event.json"), NodeType::Task);
    }

    #[test]
    fn test_build_config_classification() {
        assert_eq!(NodeType::from_path("package.json"), NodeType::Config);
        assert_eq!(NodeType::from_path("Cargo.toml"), NodeType::Config);
        assert_eq!(NodeType::from_path("pyproject.toml"), NodeType::Config);
        assert_eq!(NodeType::from_path("tsconfig.json"), NodeType::Config);
        assert_eq!(NodeType::from_path("build.gradle"), NodeType::Config);
        assert_eq!(NodeType::from_path("src/package.json"), NodeType::Config);
    }

    #[test]
    fn test_env_file_classification() {
        assert_eq!(NodeType::from_path(".env"), NodeType::Connection);
        assert_eq!(NodeType::from_path(".env.local"), NodeType::Connection);
        assert_eq!(NodeType::from_path(".env.production"), NodeType::Connection);
        assert_eq!(NodeType::from_path(".env.development"), NodeType::Connection);
        assert_eq!(NodeType::from_path("config/.env"), NodeType::Connection);
    }

    #[test]
    fn test_session_classification() {
        assert_eq!(NodeType::from_path("session.jsonl"), NodeType::Session);
        assert_eq!(NodeType::from_path("history/chat.jsonl"), NodeType::Session);
        assert_eq!(NodeType::from_path("logs/session-123.jsonl"), NodeType::Session);
    }

    #[test]
    fn test_task_classification() {
        assert_eq!(NodeType::from_path("tasks/TODO.md"), NodeType::Task);
        assert_eq!(NodeType::from_path("task-list.txt"), NodeType::Task);
    }

    #[test]
    fn test_connection_classification() {
        assert_eq!(NodeType::from_path("mcp-config.json"), NodeType::Connection);
        assert_eq!(NodeType::from_path("connection-settings.yaml"), NodeType::Connection);
    }

    #[test]
    fn test_general_config_classification() {
        assert_eq!(NodeType::from_path("config.yaml"), NodeType::Config);
        assert_eq!(NodeType::from_path("settings.json"), NodeType::Config);
        assert_eq!(NodeType::from_path("app.toml"), NodeType::Config);
    }

    #[test]
    fn test_document_classification() {
        assert_eq!(NodeType::from_path("README.md"), NodeType::Document);
        assert_eq!(NodeType::from_path("src/main.rs"), NodeType::Document);
        assert_eq!(NodeType::from_path("index.html"), NodeType::Document);
    }
}

/// Position on the canvas
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Position {
    pub x: f64,
    pub y: f64,
}

/// A node in the canvas
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct CanvasNode {
    pub id: String,
    #[serde(rename = "type")]
    pub node_type: NodeType,
    pub label: String,
    pub path: Option<String>,
    pub metadata: HashMap<String, serde_json::Value>,
    pub position: Position,
    pub timestamp: Option<i64>,
}

/// Relationship type between nodes
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "lowercase")]
pub enum RelationType {
    References,
    Creates,
    Depends,
    Applies,
    Mentions,
}

/// An edge connecting two nodes
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct CanvasEdge {
    pub id: String,
    pub source: String,
    pub target: String,
    pub relation: RelationType,
}

/// Viewport state
#[derive(Debug, Clone, Serialize, Deserialize, Default)]
pub struct Viewport {
    pub x: f64,
    pub y: f64,
    pub zoom: f64,
}

/// Complete canvas data
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct CanvasData {
    pub nodes: Vec<CanvasNode>,
    pub edges: Vec<CanvasEdge>,
    pub viewport: Viewport,
}

impl Default for CanvasData {
    fn default() -> Self {
        Self {
            nodes: Vec::new(),
            edges: Vec::new(),
            viewport: Viewport {
                x: 0.0,
                y: 0.0,
                zoom: 1.0,
            },
        }
    }
}

// --- Dashboard API types ---

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ProjectState {
    pub status: String,
    pub project_id: String,
    pub workers: Vec<WorkerInfo>,
    pub progress: TaskProgress,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct WorkerInfo {
    pub id: String,
    pub status: String,
    pub current_task: Option<String>,
    pub last_seen: Option<i64>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct TaskProgress {
    pub total: u32,
    pub done: u32,
    pub in_progress: u32,
    pub pending: u32,
    pub blocked: u32,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct TaskItem {
    pub id: String,
    pub title: String,
    pub status: String,
    pub task_type: String,
    pub dependencies: Vec<String>,
    pub assigned_to: Option<String>,
    pub domain: Option<String>,
    pub priority: i32,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct TaskDetail {
    pub id: String,
    pub title: String,
    pub status: String,
    pub task_type: String,
    pub dependencies: Vec<String>,
    pub assigned_to: Option<String>,
    pub domain: Option<String>,
    pub priority: i32,
    pub dod: String,
    pub scope: Option<String>,
    pub branch: Option<String>,
    pub commit_sha: Option<String>,
    pub version: i32,
    pub parent_id: Option<String>,
    pub review_decision: Option<String>,
    pub validations: Vec<String>,
}

// --- Session API types ---

/// Metadata for a single Claude Code session
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct SessionMeta {
    pub id: String,
    pub slug: String,
    pub title: Option<String>,
    pub path: String,
    pub line_count: u32,
    pub file_size: u64,
    pub timestamp: Option<i64>,
    pub git_branch: Option<String>,
}

/// Paginated session messages
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct SessionPage {
    pub messages: Vec<SessionMessage>,
    pub total_lines: u32,
    pub has_more: bool,
}

/// A single message in a session
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct SessionMessage {
    pub msg_type: String,
    pub timestamp: Option<String>,
    pub uuid: Option<String>,
    pub content: Vec<ContentBlock>,
}

/// A content block within a message
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ContentBlock {
    pub block_type: String,
    pub text: Option<String>,
    pub tool_name: Option<String>,
    pub tool_input: Option<serde_json::Value>,
}

/// A file change recorded in a session
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct FileChange {
    pub path: String,
    pub backup_file: Option<String>,
    pub version: Option<i32>,
    pub timestamp: Option<String>,
}

// --- Config API types ---

/// A config file entry for the explorer
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ConfigFileEntry {
    pub path: String,
    pub name: String,
    pub category: String,
    pub size: u64,
    pub modified: Option<i64>,
}

/// Content of a config file
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ConfigFileContent {
    pub path: String,
    pub content: String,
    pub truncated: bool,
}

// --- Dashboard Enhancement types (Phase 2) ---

/// A task state change event for the timeline
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct TaskEvent {
    pub task_id: String,
    pub title: String,
    pub status: String,
    pub task_type: String,
    pub updated_at: Option<String>,
    pub assigned_to: Option<String>,
}

/// A worker activity event
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct WorkerEvent {
    pub worker_id: String,
    pub task_id: String,
    pub action: String,
    pub timestamp: Option<String>,
}

/// A validation result for a task
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ValidationResult {
    pub name: String,
    pub passed: bool,
    pub output: String,
}

/// Result of scanning a project
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ScanResult {
    pub success: bool,
    pub data: Option<CanvasData>,
    pub error: Option<String>,
}

impl ScanResult {
    pub fn ok(data: CanvasData) -> Self {
        Self {
            success: true,
            data: Some(data),
            error: None,
        }
    }

    pub fn err(msg: impl Into<String>) -> Self {
        Self {
            success: false,
            data: None,
            error: Some(msg.into()),
        }
    }
}
