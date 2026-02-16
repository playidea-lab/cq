//! Knowledge — browse C9 Knowledge Store (local SQLite)
//!
//! Provides read-only Tauri IPC commands to list, search, and view
//! knowledge documents stored in `.c4/knowledge/knowledge.db`.

use std::path::Path;

use rusqlite::Connection;
use serde::{Deserialize, Serialize};

// ---------------------------------------------------------------------------
// Data models
// ---------------------------------------------------------------------------

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct KnowledgeItem {
    pub id: String,
    pub doc_type: String,
    pub title: String,
    pub domain: String,
    pub tags: Vec<String>,
    pub created_at: String,
    pub updated_at: String,
    pub version: i64,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct KnowledgeDocument {
    pub id: String,
    pub doc_type: String,
    pub title: String,
    pub domain: String,
    pub tags: Vec<String>,
    pub body: String,
    pub created_at: String,
    pub updated_at: String,
    pub version: i64,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct KnowledgeStats {
    pub total_documents: usize,
    pub by_type: Vec<TypeCount>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct TypeCount {
    pub doc_type: String,
    pub count: usize,
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

fn open_knowledge_db(project_path: &str) -> Result<Connection, String> {
    let db_path = Path::new(project_path)
        .join(".c4")
        .join("knowledge")
        .join("index.db");

    if !db_path.exists() {
        return Err(format!(
            "Knowledge database not found: {}",
            db_path.display()
        ));
    }

    Connection::open_with_flags(
        &db_path,
        rusqlite::OpenFlags::SQLITE_OPEN_READ_ONLY | rusqlite::OpenFlags::SQLITE_OPEN_NO_MUTEX,
    )
    .map_err(|e| format!("Failed to open knowledge database: {}", e))
}

fn parse_tags(tags_json: &str) -> Vec<String> {
    serde_json::from_str(tags_json).unwrap_or_default()
}

/// Read the document body from its file_path
fn read_body(file_path: &str) -> String {
    std::fs::read_to_string(file_path).unwrap_or_default()
}

// ---------------------------------------------------------------------------
// Tauri IPC commands
// ---------------------------------------------------------------------------

/// List/search knowledge documents
#[tauri::command(rename_all = "camelCase")]
pub async fn list_knowledge(
    project_path: String,
    query: Option<String>,
    doc_type: Option<String>,
    limit: Option<usize>,
) -> Result<Vec<KnowledgeItem>, String> {
    tokio::task::spawn_blocking(move || {
        let conn = open_knowledge_db(&project_path)?;
        let limit = limit.unwrap_or(100);

        // Use FTS5 search if query is provided, otherwise regular listing
        if let Some(ref q) = query {
            if !q.trim().is_empty() {
                return search_fts(&conn, q, doc_type.as_deref(), limit);
            }
        }

        // Regular listing with optional type filter
        let mut sql = String::from(
            "SELECT id, type, title, domain, tags_json, created_at, updated_at, version \
             FROM documents",
        );
        let mut params: Vec<Box<dyn rusqlite::types::ToSql>> = Vec::new();

        if let Some(ref dt) = doc_type {
            sql.push_str(" WHERE type = ?");
            params.push(Box::new(dt.clone()));
        }

        sql.push_str(" ORDER BY updated_at DESC LIMIT ?");
        params.push(Box::new(limit as i64));

        let param_refs: Vec<&dyn rusqlite::types::ToSql> =
            params.iter().map(|p| p.as_ref()).collect();

        let mut stmt = conn
            .prepare(&sql)
            .map_err(|e| format!("Query error: {}", e))?;

        let items = stmt
            .query_map(param_refs.as_slice(), |row| {
                let tags_json: String = row.get(4)?;
                Ok(KnowledgeItem {
                    id: row.get(0)?,
                    doc_type: row.get(1)?,
                    title: row.get(2)?,
                    domain: row.get(3)?,
                    tags: parse_tags(&tags_json),
                    created_at: row.get(5)?,
                    updated_at: row.get(6)?,
                    version: row.get(7)?,
                })
            })
            .map_err(|e| format!("Query error: {}", e))?
            .filter_map(|r| r.ok())
            .collect();

        Ok(items)
    })
    .await
    .map_err(|e| format!("Task execution failed: {}", e))?
}

fn search_fts(
    conn: &Connection,
    query: &str,
    doc_type: Option<&str>,
    limit: usize,
) -> Result<Vec<KnowledgeItem>, String> {
    let mut sql = String::from(
        "SELECT d.id, d.type, d.title, d.domain, d.tags_json, d.created_at, d.updated_at, d.version \
         FROM documents_fts f \
         JOIN documents d ON d.id = f.id \
         WHERE documents_fts MATCH ?",
    );
    let mut params: Vec<Box<dyn rusqlite::types::ToSql>> = Vec::new();
    params.push(Box::new(query.to_string()));

    if let Some(dt) = doc_type {
        sql.push_str(" AND d.type = ?");
        params.push(Box::new(dt.to_string()));
    }

    sql.push_str(" ORDER BY rank LIMIT ?");
    params.push(Box::new(limit as i64));

    let param_refs: Vec<&dyn rusqlite::types::ToSql> =
        params.iter().map(|p| p.as_ref()).collect();

    let mut stmt = conn
        .prepare(&sql)
        .map_err(|e| format!("FTS query error: {}", e))?;

    let items = stmt
        .query_map(param_refs.as_slice(), |row| {
            let tags_json: String = row.get(4)?;
            Ok(KnowledgeItem {
                id: row.get(0)?,
                doc_type: row.get(1)?,
                title: row.get(2)?,
                domain: row.get(3)?,
                tags: parse_tags(&tags_json),
                created_at: row.get(5)?,
                updated_at: row.get(6)?,
                version: row.get(7)?,
            })
        })
        .map_err(|e| format!("FTS query error: {}", e))?
        .filter_map(|r| r.ok())
        .collect();

    Ok(items)
}

/// Get a single knowledge document with its body
#[tauri::command(rename_all = "camelCase")]
pub async fn get_knowledge_doc(
    project_path: String,
    doc_id: String,
) -> Result<KnowledgeDocument, String> {
    tokio::task::spawn_blocking(move || {
        let conn = open_knowledge_db(&project_path)?;

        let mut stmt = conn
            .prepare(
                "SELECT id, type, title, domain, tags_json, file_path, created_at, updated_at, version \
                 FROM documents WHERE id = ?",
            )
            .map_err(|e| format!("Query error: {}", e))?;

        stmt.query_row([&doc_id], |row| {
            let tags_json: String = row.get(4)?;
            let file_path: String = row.get(5)?;
            Ok(KnowledgeDocument {
                id: row.get(0)?,
                doc_type: row.get(1)?,
                title: row.get(2)?,
                domain: row.get(3)?,
                tags: parse_tags(&tags_json),
                body: read_body(&file_path),
                created_at: row.get(6)?,
                updated_at: row.get(7)?,
                version: row.get(8)?,
            })
        })
        .map_err(|e| format!("Document not found: {}", e))
    })
    .await
    .map_err(|e| format!("Task execution failed: {}", e))?
}

/// Get knowledge store statistics
#[tauri::command(rename_all = "camelCase")]
pub async fn get_knowledge_stats(
    project_path: String,
) -> Result<KnowledgeStats, String> {
    tokio::task::spawn_blocking(move || {
        let conn = open_knowledge_db(&project_path)?;

        let total: usize = conn
            .query_row("SELECT COUNT(*) FROM documents", [], |row| row.get(0))
            .map_err(|e| format!("Count error: {}", e))?;

        let mut stmt = conn
            .prepare("SELECT type, COUNT(*) FROM documents GROUP BY type ORDER BY COUNT(*) DESC")
            .map_err(|e| format!("Stats error: {}", e))?;

        let by_type = stmt
            .query_map([], |row| {
                Ok(TypeCount {
                    doc_type: row.get(0)?,
                    count: row.get(1)?,
                })
            })
            .map_err(|e| format!("Stats error: {}", e))?
            .filter_map(|r| r.ok())
            .collect();

        Ok(KnowledgeStats {
            total_documents: total,
            by_type,
        })
    })
    .await
    .map_err(|e| format!("Task execution failed: {}", e))?
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

#[cfg(test)]
mod tests {
    use super::*;
    use std::fs;

    fn setup_test_db() -> (tempfile::TempDir, String) {
        let dir = tempfile::tempdir().unwrap();
        let path = dir.path().to_string_lossy().to_string();
        let knowledge_dir = dir.path().join(".c4").join("knowledge");
        fs::create_dir_all(&knowledge_dir).unwrap();

        let db_path = knowledge_dir.join("index.db");
        let conn = Connection::open(&db_path).unwrap();

        conn.execute_batch(
            "CREATE TABLE IF NOT EXISTS documents (
                id TEXT PRIMARY KEY,
                type TEXT NOT NULL,
                title TEXT NOT NULL DEFAULT '',
                domain TEXT DEFAULT '',
                tags_json TEXT DEFAULT '[]',
                hypothesis_status TEXT DEFAULT '',
                confidence REAL DEFAULT 0.0,
                task_id TEXT DEFAULT '',
                metadata_json TEXT DEFAULT '{}',
                file_path TEXT NOT NULL,
                content_hash TEXT NOT NULL,
                created_at TEXT NOT NULL,
                updated_at TEXT NOT NULL,
                version INTEGER DEFAULT 1
            );
            CREATE VIRTUAL TABLE IF NOT EXISTS documents_fts USING fts5(
                id, title, domain, tags_text, body_text
            );"
        ).unwrap();

        // Insert test data
        let doc_file = knowledge_dir.join("test-doc.md");
        fs::write(&doc_file, "# Test Document\n\nThis is a test.").unwrap();

        conn.execute(
            "INSERT INTO documents (id, type, title, domain, tags_json, file_path, content_hash, created_at, updated_at)
             VALUES ('doc-1', 'insight', 'Test Insight', 'testing', '[\"tag1\"]', ?1, 'hash1', '2026-02-16', '2026-02-16')",
            [doc_file.to_string_lossy().to_string()],
        ).unwrap();

        conn.execute(
            "INSERT INTO documents_fts (id, title, domain, tags_text, body_text)
             VALUES ('doc-1', 'Test Insight', 'testing', 'tag1', 'This is a test')",
            [],
        ).unwrap();

        conn.execute(
            "INSERT INTO documents (id, type, title, domain, tags_json, file_path, content_hash, created_at, updated_at)
             VALUES ('doc-2', 'pattern', 'Test Pattern', 'dev', '[]', '/tmp/none.md', 'hash2', '2026-02-16', '2026-02-16')",
            [],
        ).unwrap();

        conn.execute(
            "INSERT INTO documents_fts (id, title, domain, tags_text, body_text)
             VALUES ('doc-2', 'Test Pattern', 'dev', '', 'Some pattern')",
            [],
        ).unwrap();

        (dir, path)
    }

    #[test]
    fn test_open_knowledge_db_missing() {
        let result = open_knowledge_db("/nonexistent/path");
        assert!(result.is_err());
    }

    #[test]
    fn test_open_knowledge_db_exists() {
        let (_dir, path) = setup_test_db();
        let result = open_knowledge_db(&path);
        assert!(result.is_ok());
    }

    #[test]
    fn test_parse_tags() {
        assert_eq!(parse_tags("[\"a\",\"b\"]"), vec!["a", "b"]);
        assert_eq!(parse_tags("invalid"), Vec::<String>::new());
        assert_eq!(parse_tags("[]"), Vec::<String>::new());
    }

    #[tokio::test]
    async fn test_list_knowledge_all() {
        let (_dir, path) = setup_test_db();
        let result = list_knowledge(path, None, None, None).await;
        assert!(result.is_ok());
        let items = result.unwrap();
        assert_eq!(items.len(), 2);
    }

    #[tokio::test]
    async fn test_list_knowledge_by_type() {
        let (_dir, path) = setup_test_db();
        let result = list_knowledge(path, None, Some("insight".to_string()), None).await;
        assert!(result.is_ok());
        let items = result.unwrap();
        assert_eq!(items.len(), 1);
        assert_eq!(items[0].doc_type, "insight");
    }

    #[tokio::test]
    async fn test_list_knowledge_search() {
        let (_dir, path) = setup_test_db();
        let result = list_knowledge(path, Some("test".to_string()), None, None).await;
        assert!(result.is_ok());
        let items = result.unwrap();
        assert!(!items.is_empty());
    }

    #[tokio::test]
    async fn test_get_knowledge_doc() {
        let (_dir, path) = setup_test_db();
        let result = get_knowledge_doc(path, "doc-1".to_string()).await;
        assert!(result.is_ok());
        let doc = result.unwrap();
        assert_eq!(doc.title, "Test Insight");
        assert!(doc.body.contains("Test Document"));
    }

    #[tokio::test]
    async fn test_get_knowledge_doc_missing() {
        let (_dir, path) = setup_test_db();
        let result = get_knowledge_doc(path, "nonexistent".to_string()).await;
        assert!(result.is_err());
    }

    #[tokio::test]
    async fn test_get_knowledge_stats() {
        let (_dir, path) = setup_test_db();
        let result = get_knowledge_stats(path).await;
        assert!(result.is_ok());
        let stats = result.unwrap();
        assert_eq!(stats.total_documents, 2);
        assert_eq!(stats.by_type.len(), 2);
    }
}
