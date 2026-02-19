//! Messaging and Channels — Supabase REST client for team chat/IPC
//!
//! Provides team collaboration features via c1_channels and c1_messages tables.
//! All HTTP calls use blocking reqwest inside spawn_blocking to avoid blocking
//! the Tauri event loop.

use serde::{Deserialize, Serialize};
use base64::{Engine as _, engine::general_purpose::URL_SAFE_NO_PAD};

use crate::cloud::{build_client, read_auth_token, read_supabase_config, retry_request};

// ---------------------------------------------------------------------------
// JWT Token Helpers
// ---------------------------------------------------------------------------

/// Extract user ID from a JWT token's payload without signature verification.
///
/// SECURITY NOTE: This function performs base64 decoding of the JWT payload
/// WITHOUT signature verification. This is intentional for client-side display
/// purposes only (e.g., showing user info in UI before server validation).
///
/// This is acceptable because:
/// - The token is only used for display, not authorization
/// - Server-side validation happens when the token is used for API calls
/// - Supabase verifies the signature on all authenticated requests
///
/// DO NOT use this for authorization decisions - always verify tokens server-side.
fn extract_user_id_from_token(token: &str) -> Result<String, String> {
    // JWT format: header.payload.signature
    let parts: Vec<&str> = token.split('.').collect();
    if parts.len() != 3 {
        return Err(format!(
            "Invalid JWT format: expected 3 parts, got {}",
            parts.len()
        ));
    }

    // Decode the payload (second part)
    let payload_b64 = parts[1];

    // JWT uses base64url encoding (- instead of +, _ instead of /)
    let payload_bytes = URL_SAFE_NO_PAD
        .decode(payload_b64)
        .map_err(|e| format!("Failed to decode JWT payload: {}", e))?;

    let payload_str = String::from_utf8(payload_bytes)
        .map_err(|e| format!("JWT payload is not valid UTF-8: {}", e))?;

    // Parse JSON payload
    let payload: serde_json::Value = serde_json::from_str(&payload_str)
        .map_err(|e| format!("Failed to parse JWT payload as JSON: {}", e))?;

    // Extract 'sub' field (subject, which is the user ID in JWTs)
    let user_id = payload
        .get("sub")
        .and_then(|v| v.as_str())
        .ok_or_else(|| "JWT payload missing 'sub' field".to_string())?;

    // Validate that user_id is not empty
    if user_id.is_empty() {
        return Err("JWT 'sub' field is empty".to_string());
    }

    Ok(user_id.to_string())
}

// ---------------------------------------------------------------------------
// Data models
// ---------------------------------------------------------------------------

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Channel {
    pub id: String,
    pub project_id: String,
    pub name: String,
    #[serde(default)]
    pub description: String,
    #[serde(default)]
    pub channel_type: String,
    #[serde(default)]
    pub created_at: String,
    #[serde(default)]
    pub updated_at: String,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Message {
    pub id: String,
    pub channel_id: String,
    pub participant_id: String,
    pub content: String,
    #[serde(default)]
    pub thread_id: Option<String>,
    #[serde(default)]
    pub metadata: Option<serde_json::Value>,
    #[serde(default)]
    pub member_id: Option<String>,
    #[serde(default)]
    pub created_at: String,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Member {
    pub id: String,
    pub project_id: String,
    pub member_type: String,
    pub external_id: String,
    #[serde(default)]
    pub display_name: String,
    #[serde(default)]
    pub avatar: String,
    #[serde(default)]
    pub status: String,
    #[serde(default)]
    pub status_text: String,
    #[serde(default)]
    pub last_seen_at: Option<String>,
    #[serde(default)]
    pub created_at: String,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct MessagePage {
    pub messages: Vec<Message>,
    pub has_more: bool,
    pub total: u32,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ChannelSummary {
    pub channel_id: String,
    pub unread_count: u32,
    pub last_message_at: Option<String>,
    pub participant_count: u32,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Participant {
    pub id: String,
    pub channel_id: String,
    pub participant_id: String,
    #[serde(default)]
    pub last_read_at: Option<String>,
    #[serde(default)]
    pub joined_at: String,
}

// ---------------------------------------------------------------------------
// Tauri IPC commands
// ---------------------------------------------------------------------------

/// List all channels for a project, ordered by name
#[tauri::command(rename_all = "camelCase")]
pub async fn list_channels(project_id: String) -> Result<Vec<Channel>, String> {
    tokio::task::spawn_blocking(move || {
        let (supabase_url, anon_key) = read_supabase_config()?;
        let token = read_auth_token()?;

        let client = build_client()?;
        let url = format!(
            "{}/rest/v1/c1_channels?project_id=eq.{}&select=*&order=name",
            supabase_url.trim_end_matches('/'),
            urlencoding::encode(&project_id),
        );

        let resp = retry_request(3, || {
            client
                .get(&url)
                .header("Authorization", format!("Bearer {}", token))
                .header("apikey", &anon_key)
                .send()
        })?;

        if !resp.status().is_success() {
            let status = resp.status();
            let body = resp.text().unwrap_or_default();
            return Err(format!("Failed to list channels ({}): {}", status, body));
        }

        let channels: Vec<Channel> = resp
            .json()
            .map_err(|e| format!("Failed to parse channels: {}", e))?;

        Ok(channels)
    })
    .await
    .map_err(|e| format!("Task execution failed: {}", e))?
}

/// Get messages for a channel with pagination
#[tauri::command(rename_all = "camelCase")]
pub async fn get_channel_messages(
    channel_id: String,
    offset: u32,
    limit: u32,
) -> Result<MessagePage, String> {
    tokio::task::spawn_blocking(move || {
        let (supabase_url, anon_key) = read_supabase_config()?;
        let token = read_auth_token()?;

        let client = build_client()?;

        // Get total count via Content-Range header (Prefer: count=exact)
        let count_url = format!(
            "{}/rest/v1/c1_messages?channel_id=eq.{}&select=id&limit=0",
            supabase_url.trim_end_matches('/'),
            urlencoding::encode(&channel_id),
        );

        let count_resp = retry_request(3, || {
            client
                .get(&count_url)
                .header("Authorization", format!("Bearer {}", token))
                .header("apikey", &anon_key)
                .header("Prefer", "count=exact")
                .send()
        })?;

        let total: u32 = if count_resp.status().is_success() {
            // PostgREST returns total in Content-Range header: "*/100" or "0-9/100"
            count_resp
                .headers()
                .get("content-range")
                .and_then(|v| v.to_str().ok())
                .and_then(|s| s.split('/').last())
                .and_then(|n| n.parse().ok())
                .unwrap_or(0)
        } else {
            0
        };

        // Get paginated messages
        let messages_url = format!(
            "{}/rest/v1/c1_messages?channel_id=eq.{}&select=*&order=created_at.desc&offset={}&limit={}",
            supabase_url.trim_end_matches('/'),
            urlencoding::encode(&channel_id),
            offset,
            limit,
        );

        let resp = retry_request(3, || {
            client
                .get(&messages_url)
                .header("Authorization", format!("Bearer {}", token))
                .header("apikey", &anon_key)
                .send()
        })?;

        if !resp.status().is_success() {
            let status = resp.status();
            let body = resp.text().unwrap_or_default();
            return Err(format!("Failed to get messages ({}): {}", status, body));
        }

        let messages: Vec<Message> = resp
            .json()
            .map_err(|e| format!("Failed to parse messages: {}", e))?;

        let has_more = (offset + messages.len() as u32) < total;

        Ok(MessagePage {
            messages,
            has_more,
            total,
        })
    })
    .await
    .map_err(|e| format!("Task execution failed: {}", e))?
}

/// Send a message to a channel
#[tauri::command(rename_all = "camelCase")]
pub async fn send_message(
    channel_id: String,
    content: String,
    thread_id: Option<String>,
    metadata: Option<serde_json::Value>,
) -> Result<Message, String> {
    tokio::task::spawn_blocking(move || {
        let (supabase_url, anon_key) = read_supabase_config()?;
        let token = read_auth_token()?;

        // Get current user ID from JWT token
        let participant_id = extract_user_id_from_token(&token)?;

        let client = build_client()?;

        // Fetch project_id from channel (c1_messages.project_id is NOT NULL)
        let channel_url = format!(
            "{}/rest/v1/c1_channels?id=eq.{}&select=project_id&limit=1",
            supabase_url.trim_end_matches('/'),
            urlencoding::encode(&channel_id),
        );
        let ch_resp = retry_request(3, || {
            client
                .get(&channel_url)
                .header("Authorization", format!("Bearer {}", token))
                .header("apikey", &anon_key)
                .send()
        })?;
        if !ch_resp.status().is_success() {
            return Err(format!("Failed to fetch channel info: {}", ch_resp.status()));
        }
        let channels: Vec<serde_json::Value> = ch_resp
            .json()
            .map_err(|e| format!("Failed to parse channel: {}", e))?;
        let project_id = channels
            .first()
            .and_then(|c| c.get("project_id").and_then(|v| v.as_str()))
            .ok_or_else(|| format!("Channel {} not found", channel_id))?
            .to_string();

        let url = format!(
            "{}/rest/v1/c1_messages",
            supabase_url.trim_end_matches('/')
        );

        // Ensure user member exists (empty string → null to avoid UUID FK error)
        let member_id = ensure_user_member(&client, &supabase_url, &anon_key, &token, &participant_id)?;
        let member_id_val = if member_id.is_empty() {
            serde_json::Value::Null
        } else {
            serde_json::Value::String(member_id)
        };

        // Explicitly set participant_id to current user
        let payload = serde_json::json!({
            "channel_id": channel_id,
            "project_id": project_id,
            "participant_id": participant_id,
            "member_id": member_id_val,
            "content": content,
            "thread_id": thread_id,
            "metadata": metadata,
            "sender_type": "human",
            "sender_name": participant_id,
        });

        let resp = retry_request(3, || {
            client
                .post(&url)
                .header("Authorization", format!("Bearer {}", token))
                .header("apikey", &anon_key)
                .header("Content-Type", "application/json")
                .header("Prefer", "return=representation")
                .json(&payload)
                .send()
        })?;

        if !resp.status().is_success() {
            let status = resp.status();
            let body = resp.text().unwrap_or_default();
            return Err(format!("Failed to send message ({}): {}", status, body));
        }

        let messages: Vec<Message> = resp
            .json()
            .map_err(|e| format!("Failed to parse response: {}", e))?;

        messages
            .into_iter()
            .next()
            .ok_or_else(|| "No message returned".to_string())
    })
    .await
    .map_err(|e| format!("Task execution failed: {}", e))?
}

/// Search messages using PostgreSQL full-text search
#[tauri::command(rename_all = "camelCase")]
pub async fn search_messages(
    project_id: String,
    query: String,
    channel_id: Option<String>,
) -> Result<Vec<Message>, String> {
    tokio::task::spawn_blocking(move || {
        let (supabase_url, anon_key) = read_supabase_config()?;
        let token = read_auth_token()?;

        let client = build_client()?;

        // First, get all channels for the project to filter messages
        let channels_url = format!(
            "{}/rest/v1/c1_channels?project_id=eq.{}&select=id",
            supabase_url.trim_end_matches('/'),
            urlencoding::encode(&project_id),
        );

        let channels_resp = retry_request(3, || {
            client
                .get(&channels_url)
                .header("Authorization", format!("Bearer {}", token))
                .header("apikey", &anon_key)
                .send()
        })?;

        if !channels_resp.status().is_success() {
            return Err("Failed to fetch project channels".to_string());
        }

        let channels: Vec<serde_json::Value> = channels_resp
            .json()
            .map_err(|e| format!("Failed to parse channels: {}", e))?;

        let channel_ids: Vec<String> = channels
            .iter()
            .filter_map(|c| c.get("id").and_then(|v| v.as_str()).map(String::from))
            .collect();

        if channel_ids.is_empty() {
            return Ok(Vec::new());
        }

        // Build URL with FTS query and project-based channel filter
        let mut url = format!(
            "{}/rest/v1/c1_messages?tsv=fts(english).{}&select=*&order=created_at.desc&limit=50",
            supabase_url.trim_end_matches('/'),
            urlencoding::encode(&query),
        );

        if let Some(cid) = channel_id {
            // Filter by specific channel (must be within project)
            if channel_ids.contains(&cid) {
                url.push_str(&format!("&channel_id=eq.{}", urlencoding::encode(&cid)));
            } else {
                return Err("Channel does not belong to this project".to_string());
            }
        } else {
            // Filter by all channels in the project
            let channel_filter = channel_ids
                .iter()
                .map(|id| format!("\"{}\"", id))
                .collect::<Vec<_>>()
                .join(",");
            url.push_str(&format!("&channel_id=in.({})", channel_filter));
        }

        let resp = retry_request(3, || {
            client
                .get(&url)
                .header("Authorization", format!("Bearer {}", token))
                .header("apikey", &anon_key)
                .send()
        })?;

        if !resp.status().is_success() {
            let status = resp.status();
            let body = resp.text().unwrap_or_default();
            return Err(format!("Failed to search messages ({}): {}", status, body));
        }

        let messages: Vec<Message> = resp
            .json()
            .map_err(|e| format!("Failed to parse messages: {}", e))?;

        Ok(messages)
    })
    .await
    .map_err(|e| format!("Task execution failed: {}", e))?
}

/// Mark a channel as read (update participant's last_read_at)
#[tauri::command(rename_all = "camelCase")]
pub async fn mark_read(channel_id: String) -> Result<(), String> {
    tokio::task::spawn_blocking(move || {
        let (supabase_url, anon_key) = read_supabase_config()?;
        let token = read_auth_token()?;

        // Get current user ID from JWT token
        let participant_id = extract_user_id_from_token(&token)?;

        let client = build_client()?;
        // Filter by both channel_id and participant_id to prevent updating other users' read status
        let url = format!(
            "{}/rest/v1/c1_participants?channel_id=eq.{}&participant_id=eq.{}",
            supabase_url.trim_end_matches('/'),
            urlencoding::encode(&channel_id),
            urlencoding::encode(&participant_id),
        );

        let now = chrono::Utc::now().to_rfc3339();
        let payload = serde_json::json!({
            "last_read_at": now,
        });

        let resp = retry_request(3, || {
            client
                .patch(&url)
                .header("Authorization", format!("Bearer {}", token))
                .header("apikey", &anon_key)
                .header("Content-Type", "application/json")
                .json(&payload)
                .send()
        })?;

        if !resp.status().is_success() {
            let status = resp.status();
            let body = resp.text().unwrap_or_default();
            return Err(format!("Failed to mark read ({}): {}", status, body));
        }

        Ok(())
    })
    .await
    .map_err(|e| format!("Task execution failed: {}", e))?
}

/// Create a new channel
#[tauri::command(rename_all = "camelCase")]
pub async fn create_channel(
    project_id: String,
    name: String,
    description: String,
    channel_type: String,
) -> Result<Channel, String> {
    tokio::task::spawn_blocking(move || {
        let (supabase_url, anon_key) = read_supabase_config()?;
        let token = read_auth_token()?;

        let client = build_client()?;
        let url = format!(
            "{}/rest/v1/c1_channels",
            supabase_url.trim_end_matches('/')
        );

        let payload = serde_json::json!({
            "project_id": project_id,
            "name": name,
            "description": description,
            "channel_type": channel_type,
        });

        let resp = retry_request(3, || {
            client
                .post(&url)
                .header("Authorization", format!("Bearer {}", token))
                .header("apikey", &anon_key)
                .header("Content-Type", "application/json")
                .header("Prefer", "return=representation")
                .json(&payload)
                .send()
        })?;

        if !resp.status().is_success() {
            let status = resp.status();
            let body = resp.text().unwrap_or_default();
            return Err(format!("Failed to create channel ({}): {}", status, body));
        }

        let channels: Vec<Channel> = resp
            .json()
            .map_err(|e| format!("Failed to parse response: {}", e))?;

        channels
            .into_iter()
            .next()
            .ok_or_else(|| "No channel returned".to_string())
    })
    .await
    .map_err(|e| format!("Task execution failed: {}", e))?
}

/// Get channel summary (unread count, last message, participant count)
#[tauri::command(rename_all = "camelCase")]
pub async fn get_channel_summary(channel_id: String) -> Result<ChannelSummary, String> {
    tokio::task::spawn_blocking(move || {
        let (supabase_url, anon_key) = read_supabase_config()?;
        let token = read_auth_token()?;

        let client = build_client()?;
        let url = format!(
            "{}/rest/v1/c1_channel_summaries?channel_id=eq.{}&select=*",
            supabase_url.trim_end_matches('/'),
            urlencoding::encode(&channel_id),
        );

        let resp = retry_request(3, || {
            client
                .get(&url)
                .header("Authorization", format!("Bearer {}", token))
                .header("apikey", &anon_key)
                .send()
        })?;

        if !resp.status().is_success() {
            let status = resp.status();
            let body = resp.text().unwrap_or_default();
            return Err(format!(
                "Failed to get channel summary ({}): {}",
                status, body
            ));
        }

        let summaries: Vec<ChannelSummary> = resp
            .json()
            .map_err(|e| format!("Failed to parse summary: {}", e))?;

        summaries.into_iter().next().ok_or_else(|| {
            // Return a default summary if none exists
            format!("No summary found for channel {}", channel_id)
        })
    })
    .await
    .map_err(|e| format!("Task execution failed: {}", e))?
}

// ---------------------------------------------------------------------------
// Member helpers
// ---------------------------------------------------------------------------

/// Ensure a user member exists for the given participant_id.
/// Returns the member_id. Uses the project_id from the first channel the user can access.
fn ensure_user_member(
    client: &reqwest::blocking::Client,
    supabase_url: &str,
    anon_key: &str,
    token: &str,
    user_id: &str,
) -> Result<String, String> {
    // Check if member already exists by looking across all projects for this user
    let lookup_url = format!(
        "{}/rest/v1/c1_members?member_type=eq.user&external_id=eq.{}&select=id&limit=1",
        supabase_url.trim_end_matches('/'),
        urlencoding::encode(user_id),
    );

    let resp = retry_request(2, || {
        client
            .get(&lookup_url)
            .header("Authorization", format!("Bearer {}", token))
            .header("apikey", anon_key)
            .send()
    })?;

    if resp.status().is_success() {
        let members: Vec<serde_json::Value> = resp
            .json()
            .unwrap_or_default();
        if let Some(member) = members.first() {
            if let Some(id) = member.get("id").and_then(|v| v.as_str()) {
                return Ok(id.to_string());
            }
        }
    }

    // No existing member — but we need a project_id. Return empty for now
    // (the member will be created when a project context is available).
    Ok(String::new())
}

/// List all members for a project
#[tauri::command(rename_all = "camelCase")]
pub async fn list_members(project_id: String) -> Result<Vec<Member>, String> {
    tokio::task::spawn_blocking(move || {
        let (supabase_url, anon_key) = read_supabase_config()?;
        let token = read_auth_token()?;

        let client = build_client()?;
        let url = format!(
            "{}/rest/v1/c1_members?project_id=eq.{}&select=*&order=display_name",
            supabase_url.trim_end_matches('/'),
            urlencoding::encode(&project_id),
        );

        let resp = retry_request(3, || {
            client
                .get(&url)
                .header("Authorization", format!("Bearer {}", token))
                .header("apikey", &anon_key)
                .send()
        })?;

        if !resp.status().is_success() {
            let status = resp.status();
            let body = resp.text().unwrap_or_default();
            return Err(format!("Failed to list members ({}): {}", status, body));
        }

        let members: Vec<Member> = resp
            .json()
            .map_err(|e| format!("Failed to parse members: {}", e))?;

        Ok(members)
    })
    .await
    .map_err(|e| format!("Task execution failed: {}", e))?
}

/// Update presence status for the current user
#[tauri::command(rename_all = "camelCase")]
pub async fn update_presence(
    project_id: String,
    status: String,
    status_text: String,
) -> Result<(), String> {
    tokio::task::spawn_blocking(move || {
        let (supabase_url, anon_key) = read_supabase_config()?;
        let token = read_auth_token()?;
        let user_id = extract_user_id_from_token(&token)?;

        let client = build_client()?;
        let url = format!(
            "{}/rest/v1/c1_members?project_id=eq.{}&member_type=eq.user&external_id=eq.{}",
            supabase_url.trim_end_matches('/'),
            urlencoding::encode(&project_id),
            urlencoding::encode(&user_id),
        );

        let now = chrono::Utc::now().to_rfc3339();
        let payload = serde_json::json!({
            "status": status,
            "status_text": status_text,
            "last_seen_at": now,
        });

        let resp = retry_request(3, || {
            client
                .patch(&url)
                .header("Authorization", format!("Bearer {}", token))
                .header("apikey", &anon_key)
                .header("Content-Type", "application/json")
                .json(&payload)
                .send()
        })?;

        if !resp.status().is_success() {
            let status = resp.status();
            let body = resp.text().unwrap_or_default();
            return Err(format!("Failed to update presence ({}): {}", status, body));
        }

        Ok(())
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

    #[test]
    fn test_channel_serialization() {
        let channel = Channel {
            id: "ch-1".to_string(),
            project_id: "proj-1".to_string(),
            name: "General".to_string(),
            description: "Team chat".to_string(),
            channel_type: "chat".to_string(),
            created_at: "2026-02-14T00:00:00Z".to_string(),
            updated_at: "2026-02-14T00:00:00Z".to_string(),
        };
        let json = serde_json::to_string(&channel).unwrap();
        let parsed: Channel = serde_json::from_str(&json).unwrap();
        assert_eq!(parsed.name, "General");
        assert_eq!(parsed.channel_type, "chat");
    }

    #[test]
    fn test_message_serialization() {
        let message = Message {
            id: "msg-1".to_string(),
            channel_id: "ch-1".to_string(),
            participant_id: "user-1".to_string(),
            content: "Hello world".to_string(),
            thread_id: None,
            metadata: None,
            member_id: None,
            created_at: "2026-02-14T00:00:00Z".to_string(),
        };
        let json = serde_json::to_string(&message).unwrap();
        let parsed: Message = serde_json::from_str(&json).unwrap();
        assert_eq!(parsed.content, "Hello world");
        assert!(parsed.thread_id.is_none());
    }

    #[test]
    fn test_message_with_thread() {
        let message = Message {
            id: "msg-2".to_string(),
            channel_id: "ch-1".to_string(),
            participant_id: "user-2".to_string(),
            content: "Reply".to_string(),
            thread_id: Some("msg-1".to_string()),
            metadata: Some(serde_json::json!({"emoji": "👍"})),
            member_id: None,
            created_at: "2026-02-14T00:01:00Z".to_string(),
        };
        let json = serde_json::to_string(&message).unwrap();
        let parsed: Message = serde_json::from_str(&json).unwrap();
        assert_eq!(parsed.thread_id, Some("msg-1".to_string()));
        assert!(parsed.metadata.is_some());
    }

    #[test]
    fn test_message_page_serialization() {
        let page = MessagePage {
            messages: vec![],
            has_more: true,
            total: 100,
        };
        let json = serde_json::to_string(&page).unwrap();
        let parsed: MessagePage = serde_json::from_str(&json).unwrap();
        assert!(parsed.has_more);
        assert_eq!(parsed.total, 100);
    }

    #[test]
    fn test_channel_summary_serialization() {
        let summary = ChannelSummary {
            channel_id: "ch-1".to_string(),
            unread_count: 5,
            last_message_at: Some("2026-02-14T00:00:00Z".to_string()),
            participant_count: 10,
        };
        let json = serde_json::to_string(&summary).unwrap();
        let parsed: ChannelSummary = serde_json::from_str(&json).unwrap();
        assert_eq!(parsed.unread_count, 5);
        assert_eq!(parsed.participant_count, 10);
    }

    #[test]
    fn test_participant_serialization() {
        let participant = Participant {
            id: "p-1".to_string(),
            channel_id: "ch-1".to_string(),
            participant_id: "user-1".to_string(),
            last_read_at: Some("2026-02-14T00:00:00Z".to_string()),
            joined_at: "2026-02-13T00:00:00Z".to_string(),
        };
        let json = serde_json::to_string(&participant).unwrap();
        let parsed: Participant = serde_json::from_str(&json).unwrap();
        assert_eq!(parsed.participant_id, "user-1");
        assert!(parsed.last_read_at.is_some());
    }

    #[test]
    fn test_extract_user_id_valid_token() {
        // Valid JWT with sub field: header.payload.signature
        // Payload: {"sub":"user-123","exp":1234567890}
        let payload = r#"{"sub":"user-123","exp":1234567890}"#;
        let payload_b64 = URL_SAFE_NO_PAD.encode(payload.as_bytes());
        let token = format!("eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.{}.fake_signature", payload_b64);

        let result = extract_user_id_from_token(&token);
        assert!(result.is_ok());
        assert_eq!(result.unwrap(), "user-123");
    }

    #[test]
    fn test_extract_user_id_malformed_token() {
        // Test various malformed tokens
        let test_cases = vec![
            ("not.a.valid.jwt.extra", "expected 3 parts, got 5"),
            ("only.two", "expected 3 parts, got 2"),
            ("", "expected 3 parts, got 1"),
            ("a.invalid_base64!.c", "Failed to decode JWT payload"),
        ];

        for (token, expected_error) in test_cases {
            let result = extract_user_id_from_token(token);
            assert!(result.is_err(), "Expected error for token: {}", token);
            assert!(
                result.unwrap_err().contains(expected_error),
                "Expected error containing '{}'",
                expected_error
            );
        }
    }

    #[test]
    fn test_extract_user_id_missing_sub_field() {
        // JWT payload without 'sub' field
        let payload = r#"{"exp":1234567890,"iat":1234567890}"#;
        let payload_b64 = URL_SAFE_NO_PAD.encode(payload.as_bytes());
        let token = format!("eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.{}.fake_sig", payload_b64);

        let result = extract_user_id_from_token(&token);
        assert!(result.is_err());
        assert!(result.unwrap_err().contains("missing 'sub' field"));
    }

    #[test]
    fn test_extract_user_id_empty_sub() {
        // JWT payload with empty 'sub' field
        let payload = r#"{"sub":"","exp":1234567890}"#;
        let payload_b64 = URL_SAFE_NO_PAD.encode(payload.as_bytes());
        let token = format!("eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.{}.fake_sig", payload_b64);

        let result = extract_user_id_from_token(&token);
        assert!(result.is_err());
        assert!(result.unwrap_err().contains("'sub' field is empty"));
    }

    #[test]
    fn test_extract_user_id_invalid_json() {
        // JWT payload with invalid JSON
        let payload = "not json at all";
        let payload_b64 = URL_SAFE_NO_PAD.encode(payload.as_bytes());
        let token = format!("eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.{}.fake_sig", payload_b64);

        let result = extract_user_id_from_token(&token);
        assert!(result.is_err());
        assert!(result.unwrap_err().contains("Failed to parse JWT payload"));
    }

    #[test]
    fn test_member_serialization() {
        let member = Member {
            id: "mem-1".to_string(),
            project_id: "proj-1".to_string(),
            member_type: "agent".to_string(),
            external_id: "worker-1".to_string(),
            display_name: "Worker 1".to_string(),
            avatar: "".to_string(),
            status: "working".to_string(),
            status_text: "T-003-0".to_string(),
            last_seen_at: Some("2026-02-16T00:00:00Z".to_string()),
            created_at: "2026-02-16T00:00:00Z".to_string(),
        };
        let json = serde_json::to_string(&member).unwrap();
        let parsed: Member = serde_json::from_str(&json).unwrap();
        assert_eq!(parsed.member_type, "agent");
        assert_eq!(parsed.status, "working");
        assert_eq!(parsed.display_name, "Worker 1");
    }

    #[test]
    fn test_message_with_member_id() {
        let message = Message {
            id: "msg-3".to_string(),
            channel_id: "ch-1".to_string(),
            participant_id: "user-1".to_string(),
            content: "Hello with member".to_string(),
            thread_id: None,
            metadata: None,
            member_id: Some("mem-1".to_string()),
            created_at: "2026-02-16T00:00:00Z".to_string(),
        };
        let json = serde_json::to_string(&message).unwrap();
        let parsed: Message = serde_json::from_str(&json).unwrap();
        assert_eq!(parsed.member_id, Some("mem-1".to_string()));
    }

    #[test]
    fn test_message_without_member_id() {
        // Legacy message without member_id
        let json = r#"{"id":"msg-4","channel_id":"ch-1","participant_id":"user-1","content":"legacy","created_at":"2026-02-16T00:00:00Z"}"#;
        let parsed: Message = serde_json::from_str(json).unwrap();
        assert!(parsed.member_id.is_none());
    }
}
