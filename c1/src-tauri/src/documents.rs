//! Documents — local file management for personas, skills, specs, and configs
//!
//! Provides Tauri IPC commands to list, read, and save project documents.
//! All file I/O uses spawn_blocking to avoid blocking the Tauri event loop.

use std::fs;
use std::path::{Path, PathBuf};
use std::time::UNIX_EPOCH;

use serde::{Deserialize, Serialize};

// ---------------------------------------------------------------------------
// Data models
// ---------------------------------------------------------------------------

/// Validate a document path to prevent path traversal attacks
fn validate_document_path(path: &str, allowed_base: &Path) -> Result<PathBuf, String> {
    // Check for ".." components before canonicalization
    if path.contains("..") {
        return Err("Path contains '..' which is not allowed".to_string());
    }

    let path_buf = PathBuf::from(path);

    // Canonicalize to resolve symlinks and relative paths
    let canonical_path = path_buf
        .canonicalize()
        .map_err(|e| format!("Failed to canonicalize path: {}", e))?;

    // Ensure the canonical path is within the allowed base directory
    let canonical_base = allowed_base
        .canonicalize()
        .map_err(|e| format!("Failed to canonicalize base directory: {}", e))?;

    if !canonical_path.starts_with(&canonical_base) {
        return Err(format!(
            "Path '{}' is outside allowed directory '{}'",
            canonical_path.display(),
            canonical_base.display()
        ));
    }

    Ok(canonical_path)
}

// ---------------------------------------------------------------------------
// Data models (continued)
// ---------------------------------------------------------------------------

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct DocumentMeta {
    pub name: String,
    pub doc_type: String,
    pub path: String,
    pub size: u64,
    pub updated_at: Option<String>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct DocumentContent {
    pub name: String,
    pub doc_type: String,
    pub content: String,
    pub path: String,
    pub updated_at: Option<String>,
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

fn file_meta(path: &Path, doc_type: &str) -> Option<DocumentMeta> {
    let metadata = fs::metadata(path).ok()?;
    let name = path.file_name()?.to_string_lossy().to_string();
    let updated_at = metadata
        .modified()
        .ok()
        .and_then(|t| t.duration_since(UNIX_EPOCH).ok())
        .map(|d| {
            let secs = d.as_secs() as i64;
            chrono::DateTime::from_timestamp(secs, 0)
                .map(|dt| dt.to_rfc3339())
                .unwrap_or_default()
        });

    Some(DocumentMeta {
        name,
        doc_type: doc_type.to_string(),
        path: path.to_string_lossy().to_string(),
        size: metadata.len(),
        updated_at,
    })
}

fn scan_directory(dir: &Path, extension: &str, doc_type: &str) -> Vec<DocumentMeta> {
    let mut docs = Vec::new();
    if let Ok(entries) = fs::read_dir(dir) {
        for entry in entries.flatten() {
            let path = entry.path();
            if path.is_file() && path.extension().map_or(false, |ext| ext == extension) {
                if let Some(meta) = file_meta(&path, doc_type) {
                    docs.push(meta);
                }
            }
        }
    }
    docs.sort_by(|a, b| a.name.cmp(&b.name));
    docs
}

/// Resolve document paths for each doc_type based on project_path.
fn resolve_paths(project_path: &str, doc_type: &str) -> Vec<PathBuf> {
    let project = Path::new(project_path);
    let home = dirs::home_dir().unwrap_or_default();

    match doc_type {
        "persona" => vec![
            home.join(".claude").join("agents"),
            project.join(".c4"),
        ],
        "skill" => vec![
            project.join(".claude").join("commands"),
            home.join(".claude").join("commands"),
        ],
        "spec" => vec![
            project.join(".c4").join("specs"),
        ],
        "config" => vec![
            project.join(".claude"),
            project.join(".c4"),
        ],
        _ => vec![],
    }
}

// ---------------------------------------------------------------------------
// Tauri IPC commands
// ---------------------------------------------------------------------------

/// List documents of a given type for a project
#[tauri::command(rename_all = "camelCase")]
pub async fn list_documents(
    project_path: String,
    doc_type: String,
) -> Result<Vec<DocumentMeta>, String> {
    tokio::task::spawn_blocking(move || {
        let mut all_docs = Vec::new();
        let dirs = resolve_paths(&project_path, &doc_type);

        for dir in &dirs {
            match doc_type.as_str() {
                "persona" => {
                    // .md files in agent dirs + SOUL.md
                    let mut docs = scan_directory(dir, "md", "persona");
                    // Also check for SOUL.md directly
                    let soul_path = dir.join("SOUL.md");
                    if soul_path.exists() {
                        if let Some(meta) = file_meta(&soul_path, "persona") {
                            if !docs.iter().any(|d| d.path == meta.path) {
                                docs.push(meta);
                            }
                        }
                    }
                    all_docs.extend(docs);
                }
                "skill" => {
                    all_docs.extend(scan_directory(dir, "md", "skill"));
                }
                "spec" => {
                    // YAML and MD specs
                    let mut specs = scan_directory(dir, "yaml", "spec");
                    specs.extend(scan_directory(dir, "yml", "spec"));
                    specs.extend(scan_directory(dir, "md", "spec"));
                    all_docs.extend(specs);
                }
                "config" => {
                    // Specific config files
                    for name in &["CLAUDE.md", "AGENTS.md", "config.yaml", "config.yml"] {
                        let path = dir.join(name);
                        if path.exists() {
                            if let Some(meta) = file_meta(&path, "config") {
                                all_docs.push(meta);
                            }
                        }
                    }
                }
                _ => {}
            }
        }

        // Deduplicate by path
        all_docs.sort_by(|a, b| a.path.cmp(&b.path));
        all_docs.dedup_by(|a, b| a.path == b.path);

        Ok(all_docs)
    })
    .await
    .map_err(|e| format!("Task execution failed: {}", e))?
}

/// Read a document's content by its file path
#[tauri::command(rename_all = "camelCase")]
pub async fn get_document(path: String) -> Result<DocumentContent, String> {
    tokio::task::spawn_blocking(move || {
        // Validate path to prevent path traversal
        let home = dirs::home_dir().unwrap_or_default();
        let allowed_base = home.join(".claude");
        let validated_path = validate_document_path(&path, &allowed_base)
            .or_else(|_| {
                // Also allow paths in current working directory
                let cwd = std::env::current_dir().unwrap_or_default();
                validate_document_path(&path, &cwd)
            })?;

        let file_path = validated_path.as_path();
        if !file_path.exists() {
            return Err(format!("File not found: {}", path));
        }

        let content = fs::read_to_string(file_path)
            .map_err(|e| format!("Failed to read {}: {}", path, e))?;

        let name = file_path
            .file_name()
            .map(|n| n.to_string_lossy().to_string())
            .unwrap_or_default();

        let doc_type = infer_doc_type(&path);

        let updated_at = fs::metadata(file_path)
            .ok()
            .and_then(|m| m.modified().ok())
            .and_then(|t| t.duration_since(UNIX_EPOCH).ok())
            .and_then(|d| {
                chrono::DateTime::from_timestamp(d.as_secs() as i64, 0)
                    .map(|dt| dt.to_rfc3339())
            });

        Ok(DocumentContent {
            name,
            doc_type,
            content,
            path,
            updated_at,
        })
    })
    .await
    .map_err(|e| format!("Task execution failed: {}", e))?
}

/// Save a document by its file path
#[tauri::command(rename_all = "camelCase")]
pub async fn save_document(path: String, content: String) -> Result<(), String> {
    tokio::task::spawn_blocking(move || {
        // Validate path to prevent path traversal
        let home = dirs::home_dir().unwrap_or_default();
        let allowed_base = home.join(".claude");

        // For new files, we need to check the parent directory instead
        let path_buf = PathBuf::from(&path);
        let parent = path_buf.parent().ok_or("Invalid path: no parent directory")?;

        // Ensure parent directory exists and is valid
        if !parent.exists() {
            fs::create_dir_all(parent)
                .map_err(|e| format!("Failed to create directory: {}", e))?;
        }

        // Validate the full path against allowed base
        let validated_path = validate_document_path(&path, &allowed_base)
            .or_else(|_| {
                // Also allow paths in current working directory
                let cwd = std::env::current_dir().unwrap_or_default();
                validate_document_path(&path, &cwd)
            })?;

        fs::write(&validated_path, &content)
            .map_err(|e| format!("Failed to write {}: {}", path, e))?;

        Ok(())
    })
    .await
    .map_err(|e| format!("Task execution failed: {}", e))?
}

fn infer_doc_type(path: &str) -> String {
    if path.contains("agents") || path.contains("SOUL") {
        "persona".to_string()
    } else if path.contains("commands") {
        "skill".to_string()
    } else if path.contains("specs") {
        "spec".to_string()
    } else {
        "config".to_string()
    }
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

#[cfg(test)]
mod tests {
    use super::*;
    use std::fs;

    #[test]
    fn test_file_meta() {
        let dir = tempfile::tempdir().unwrap();
        let file = dir.path().join("test.md");
        fs::write(&file, "# Test").unwrap();

        let meta = file_meta(&file, "skill").unwrap();
        assert_eq!(meta.name, "test.md");
        assert_eq!(meta.doc_type, "skill");
        assert_eq!(meta.size, 6);
        assert!(meta.updated_at.is_some());
    }

    #[test]
    fn test_file_meta_missing() {
        let result = file_meta(Path::new("/nonexistent/file.md"), "test");
        assert!(result.is_none());
    }

    #[test]
    fn test_scan_directory() {
        let dir = tempfile::tempdir().unwrap();
        fs::write(dir.path().join("a.md"), "aaa").unwrap();
        fs::write(dir.path().join("b.md"), "bbb").unwrap();
        fs::write(dir.path().join("c.txt"), "ccc").unwrap(); // wrong extension

        let docs = scan_directory(dir.path(), "md", "skill");
        assert_eq!(docs.len(), 2);
        assert_eq!(docs[0].name, "a.md");
        assert_eq!(docs[1].name, "b.md");
    }

    #[test]
    fn test_scan_empty_directory() {
        let dir = tempfile::tempdir().unwrap();
        let docs = scan_directory(dir.path(), "md", "persona");
        assert!(docs.is_empty());
    }

    #[test]
    fn test_scan_nonexistent_directory() {
        let docs = scan_directory(Path::new("/nonexistent/path"), "md", "spec");
        assert!(docs.is_empty());
    }

    #[test]
    fn test_infer_doc_type() {
        assert_eq!(infer_doc_type("/home/.claude/agents/reviewer.md"), "persona");
        assert_eq!(infer_doc_type("/project/.c4/SOUL.md"), "persona");
        assert_eq!(infer_doc_type("/project/.claude/commands/c4-run.md"), "skill");
        assert_eq!(infer_doc_type("/project/.c4/specs/api.yaml"), "spec");
        assert_eq!(infer_doc_type("/project/.claude/CLAUDE.md"), "config");
    }

    #[test]
    fn test_resolve_paths_persona() {
        let paths = resolve_paths("/project", "persona");
        assert_eq!(paths.len(), 2);
        assert!(paths[0].to_string_lossy().contains("agents"));
        assert!(paths[1].to_string_lossy().contains(".c4"));
    }

    #[test]
    fn test_resolve_paths_unknown() {
        let paths = resolve_paths("/project", "unknown");
        assert!(paths.is_empty());
    }
}
