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
        if lower.contains(".claude") || lower.contains(".c4") || lower.ends_with(".yaml") || lower.ends_with(".json") || lower.ends_with(".toml") {
            Self::Config
        } else if lower.contains("session") || lower.contains("history") || lower.ends_with(".jsonl") {
            Self::Session
        } else if lower.contains("task") {
            Self::Task
        } else if lower.contains("mcp") || lower.contains("env") || lower.contains("connection") {
            Self::Connection
        } else {
            Self::Document
        }
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
