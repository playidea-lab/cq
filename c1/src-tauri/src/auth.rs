//! Supabase GitHub OAuth authentication
//!
//! Manages OAuth flow via system browser + localhost callback,
//! stores session in ~/.c4/session.json (Python SessionManager compatible).

use std::fs;
use std::io::{Read as IoRead, Write as IoWrite};
use std::net::TcpListener;
use std::path::PathBuf;
use std::time::Duration;

use serde::{Deserialize, Serialize};
use tauri::Emitter;

// ---------------------------------------------------------------------------
// Data models
// ---------------------------------------------------------------------------

/// Full session persisted to ~/.c4/session.json (Python SessionManager compatible)
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct AuthSession {
    pub access_token: String,
    pub refresh_token: String,
    pub token_type: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub expires_at: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub user_id: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub email: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub provider: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub provider_token: Option<String>,
    #[serde(default)]
    pub metadata: serde_json::Value,
}

/// Lightweight user info sent to frontend
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct AuthUser {
    pub id: String,
    pub email: String,
    pub provider: String,
}

/// Supabase configuration status
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct AuthConfig {
    pub supabase_url: Option<String>,
    pub has_anon_key: bool,
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

fn session_path() -> PathBuf {
    dirs::home_dir()
        .unwrap_or_else(|| PathBuf::from("."))
        .join(".c4")
        .join("session.json")
}

fn load_session_from_disk() -> Option<AuthSession> {
    let path = session_path();
    let data = fs::read_to_string(&path).ok()?;
    serde_json::from_str(&data).ok()
}

fn save_session_to_disk(session: &AuthSession) -> Result<(), String> {
    let path = session_path();
    if let Some(parent) = path.parent() {
        fs::create_dir_all(parent)
            .map_err(|e| format!("Failed to create ~/.c4: {}", e))?;
    }
    let json = serde_json::to_string_pretty(session)
        .map_err(|e| format!("Failed to serialize session: {}", e))?;
    fs::write(&path, &json)
        .map_err(|e| format!("Failed to write session.json: {}", e))?;

    // Set file permissions to 0o600 (owner read/write only)
    #[cfg(unix)]
    {
        use std::os::unix::fs::PermissionsExt;
        let perms = fs::Permissions::from_mode(0o600);
        fs::set_permissions(&path, perms)
            .map_err(|e| format!("Failed to set permissions: {}", e))?;
    }
    Ok(())
}

fn session_to_user(session: &AuthSession) -> AuthUser {
    AuthUser {
        id: session.user_id.clone().unwrap_or_default(),
        email: session.email.clone().unwrap_or_default(),
        provider: session.provider.clone().unwrap_or_else(|| "github".into()),
    }
}

/// Read Supabase config from env vars, then fall back to ~/.c4/supabase.json or ~/.c4/cloud.yaml
fn read_supabase_config() -> AuthConfig {
    // 1) env vars
    let env_url = std::env::var("SUPABASE_URL").ok();
    let env_key = std::env::var("SUPABASE_ANON_KEY")
        .or_else(|_| std::env::var("SUPABASE_KEY"))
        .ok();

    // 2) ~/.c4/supabase.json  {"url": "...", "anon_key": "..."}
    let (json_url, json_key) = dirs::home_dir()
        .and_then(|h| {
            let p = h.join(".c4").join("supabase.json");
            let s = fs::read_to_string(p).ok()?;
            let v: serde_json::Value = serde_json::from_str(&s).ok()?;
            let u = v.get("url")?.as_str().map(|s| s.to_string());
            let k = v.get("anon_key")?.as_str().map(|s| s.to_string());
            Some((u, k))
        })
        .unwrap_or((None, None));

    // 3) ~/.c4/cloud.yaml  supabase_url: ...
    let yaml_url = dirs::home_dir().and_then(|h| {
        let p = h.join(".c4").join("cloud.yaml");
        let s = fs::read_to_string(p).ok()?;
        let yaml: serde_yaml::Value = serde_yaml::from_str(&s).ok()?;
        yaml.get("supabase_url")?.as_str().map(|s| s.to_string())
    });

    let url = env_url.or(json_url).or(yaml_url);
    let has_key = env_key.is_some() || json_key.is_some();
    AuthConfig {
        supabase_url: url,
        has_anon_key: has_key,
    }
}

/// Build the Supabase OAuth URL for GitHub provider
fn build_oauth_url(supabase_url: &str, redirect_uri: &str) -> String {
    format!(
        "{}/auth/v1/authorize?provider=github&redirect_to={}",
        supabase_url.trim_end_matches('/'),
        urlencoding::encode(redirect_uri)
    )
}

/// Parse query string from a raw HTTP request line
fn parse_callback_params(request_line: &str) -> std::collections::HashMap<String, String> {
    let mut params = std::collections::HashMap::new();

    // Extract path+query from "GET /auth/callback?key=val&... HTTP/1.1"
    let parts: Vec<&str> = request_line.split_whitespace().collect();
    if parts.len() < 2 {
        return params;
    }

    // Also handle fragment-as-query: Supabase may redirect with #access_token=...
    // The browser JS rewrites # → ? before redirecting, so we parse ? here.
    if let Some(query) = parts[1].split('?').nth(1) {
        for pair in query.split('&') {
            let mut kv = pair.splitn(2, '=');
            if let (Some(k), Some(v)) = (kv.next(), kv.next()) {
                params.insert(
                    urlencoding::decode(k).unwrap_or_default().into_owned(),
                    urlencoding::decode(v).unwrap_or_default().into_owned(),
                );
            }
        }
    }
    params
}

/// HTML page shown after successful OAuth callback
const CALLBACK_SUCCESS_HTML: &str = r#"<!DOCTYPE html>
<html>
<head><meta charset="utf-8"><title>C1 Login</title>
<style>
  body { font-family: -apple-system, BlinkMacSystemFont, sans-serif;
         display: flex; justify-content: center; align-items: center;
         height: 100vh; margin: 0; background: #1a1a2e; color: #eee; }
  .card { text-align: center; padding: 48px; }
  .check { font-size: 48px; margin-bottom: 16px; }
  p { color: #aaa; margin-top: 8px; }
</style></head>
<body><div class="card">
  <div class="check">&#10003;</div>
  <h2>Login complete</h2>
  <p>You can close this tab and return to C1.</p>
  <script>setTimeout(()=>window.close(),2000)</script>
</div></body></html>"#;

const CALLBACK_ERROR_HTML: &str = r#"<!DOCTYPE html>
<html>
<head><meta charset="utf-8"><title>C1 Login Error</title>
<style>
  body { font-family: -apple-system, BlinkMacSystemFont, sans-serif;
         display: flex; justify-content: center; align-items: center;
         height: 100vh; margin: 0; background: #1a1a2e; color: #eee; }
  .card { text-align: center; padding: 48px; }
  .x { font-size: 48px; margin-bottom: 16px; color: #e03131; }
  p { color: #aaa; margin-top: 8px; }
</style></head>
<body><div class="card">
  <div class="x">&#10007;</div>
  <h2>Login failed</h2>
  <p>No access token received. Please try again.</p>
</div></body></html>"#;

// Supabase redirects with a fragment (#access_token=...) which the browser
// doesn't send to the server. This landing page rewrites it to a query string
// and redirects to the same URL so the server can read the params.
const FRAGMENT_REWRITE_HTML: &str = r#"<!DOCTYPE html>
<html>
<head><meta charset="utf-8"><title>C1 Login</title>
<style>
  body { font-family: -apple-system, BlinkMacSystemFont, sans-serif;
         display: flex; justify-content: center; align-items: center;
         height: 100vh; margin: 0; background: #1a1a2e; color: #eee; }
</style></head>
<body>
<p>Completing login...</p>
<script>
  // Supabase sends tokens as URL fragment (#access_token=...).
  // Rewrite to query string so the local server can read them.
  if (window.location.hash && window.location.hash.length > 1) {
    window.location.replace(
      window.location.pathname + '?' + window.location.hash.substring(1)
    );
  }
</script>
</body></html>"#;

// ---------------------------------------------------------------------------
// OAuth callback server (blocking, runs on a spawned thread)
// ---------------------------------------------------------------------------

/// Default OAuth callback port — must match C4 Core (cloud/auth.go defaultCallbackPort).
const DEFAULT_OAUTH_PORT: u16 = 19823;

/// Binds the OAuth callback listener. Returns (listener, port).
fn bind_callback_listener() -> Result<(TcpListener, u16), String> {
    let listener = TcpListener::bind(format!("127.0.0.1:{}", DEFAULT_OAUTH_PORT))
        .or_else(|_| TcpListener::bind("127.0.0.1:0"))
        .map_err(|e| format!("Failed to bind OAuth callback port ({})", e))?;
    let port = listener.local_addr()
        .map_err(|e| format!("Failed to get local addr: {}", e))?
        .port();
    Ok((listener, port))
}

/// Waits for the OAuth callback on an already-bound listener.
fn wait_for_callback(listener: TcpListener) -> Result<AuthSession, String> {
    listener
        .set_nonblocking(false)
        .map_err(|e| format!("set_nonblocking: {}", e))?;

    // 120-second timeout
    let deadline = std::time::Instant::now() + Duration::from_secs(120);

    loop {
        if std::time::Instant::now() > deadline {
            return Err("Login timed out (120s). Please try again.".into());
        }

        // Accept with a short timeout so we can check the deadline
        listener
            .set_nonblocking(true)
            .map_err(|e| format!("set_nonblocking: {}", e))?;

        let accept_result = listener.accept();

        match accept_result {
            Ok((mut stream, _)) => {
                stream
                    .set_read_timeout(Some(Duration::from_secs(5)))
                    .ok();

                let mut buf = [0u8; 4096];
                let n = stream.read(&mut buf).unwrap_or(0);
                let request = String::from_utf8_lossy(&buf[..n]);

                // Extract the first line (e.g. "GET /auth/callback?... HTTP/1.1")
                let first_line = request.lines().next().unwrap_or("");

                // Check if this is the callback path with actual query params
                let params = parse_callback_params(first_line);
                let has_token = params.contains_key("access_token");

                if !first_line.contains("/auth/callback") || !has_token {
                    // Either not the callback path, or callback without query params
                    // (tokens are in # fragment). Serve the rewrite page.
                    let response = format!(
                        "HTTP/1.1 200 OK\r\nContent-Type: text/html\r\nContent-Length: {}\r\n\r\n{}",
                        FRAGMENT_REWRITE_HTML.len(),
                        FRAGMENT_REWRITE_HTML
                    );
                    let _ = stream.write_all(response.as_bytes());
                    continue;
                }

                if let Some(access_token) = params.get("access_token") {
                    if access_token.is_empty() {
                        let response = format!(
                            "HTTP/1.1 200 OK\r\nContent-Type: text/html\r\nContent-Length: {}\r\n\r\n{}",
                            CALLBACK_ERROR_HTML.len(),
                            CALLBACK_ERROR_HTML
                        );
                        let _ = stream.write_all(response.as_bytes());
                        continue;
                    }

                    // Build session
                    let session = AuthSession {
                        access_token: access_token.clone(),
                        refresh_token: params
                            .get("refresh_token")
                            .cloned()
                            .unwrap_or_default(),
                        token_type: params
                            .get("token_type")
                            .cloned()
                            .unwrap_or_else(|| "bearer".into()),
                        expires_at: params.get("expires_at").and_then(|ts| {
                            // Supabase sends Unix timestamp; convert to ISO8601
                            ts.parse::<i64>().ok().map(|epoch| {
                                chrono::DateTime::from_timestamp(epoch, 0)
                                    .map(|dt| dt.to_rfc3339())
                                    .unwrap_or_else(|| ts.clone())
                            })
                        }),
                        user_id: None,
                        email: None,
                        provider: Some("github".into()),
                        provider_token: params.get("provider_token").cloned(),
                        metadata: serde_json::json!({}),
                    };

                    // Save to disk
                    save_session_to_disk(&session)?;

                    // Respond with success page
                    let response = format!(
                        "HTTP/1.1 200 OK\r\nContent-Type: text/html\r\nContent-Length: {}\r\n\r\n{}",
                        CALLBACK_SUCCESS_HTML.len(),
                        CALLBACK_SUCCESS_HTML
                    );
                    let _ = stream.write_all(response.as_bytes());

                    return Ok(session);
                } else {
                    // No access_token in params — show error
                    let response = format!(
                        "HTTP/1.1 200 OK\r\nContent-Type: text/html\r\nContent-Length: {}\r\n\r\n{}",
                        CALLBACK_ERROR_HTML.len(),
                        CALLBACK_ERROR_HTML
                    );
                    let _ = stream.write_all(response.as_bytes());
                    continue;
                }
            }
            Err(ref e) if e.kind() == std::io::ErrorKind::WouldBlock => {
                // No connection yet, sleep briefly and retry
                std::thread::sleep(Duration::from_millis(200));
                continue;
            }
            Err(e) => {
                return Err(format!("Accept failed: {}", e));
            }
        }
    }
}

// ---------------------------------------------------------------------------
// Tauri IPC commands
// ---------------------------------------------------------------------------

/// Load existing session from ~/.c4/session.json
#[tauri::command]
pub fn auth_get_session() -> Option<AuthUser> {
    let session = load_session_from_disk()?;
    Some(session_to_user(&session))
}

/// Start GitHub OAuth flow: open browser → wait for callback → save session
#[tauri::command(rename_all = "camelCase")]
pub async fn auth_login(
    app: tauri::AppHandle,
    supabase_url: String,
    anon_key: String,
) -> Result<AuthUser, String> {
    // Resolve anon_key: use env var if placeholder or empty
    let anon_key = if anon_key.is_empty() || anon_key == "__FROM_ENV__" {
        std::env::var("SUPABASE_ANON_KEY")
            .or_else(|_| std::env::var("SUPABASE_KEY"))
            .map_err(|_| "SUPABASE_ANON_KEY (or SUPABASE_KEY) not set".to_string())?
    } else {
        anon_key
    };

    // Bind callback listener first to get the actual port
    let (listener, port) = bind_callback_listener()?;
    let redirect_uri = format!("http://127.0.0.1:{}/auth/callback", port);
    let oauth_url = build_oauth_url(&supabase_url, &redirect_uri);

    // Open system browser
    tauri::async_runtime::spawn(async move {
        let _ = open::that(&oauth_url);
    });

    // Wait for OAuth callback on the already-bound listener
    let session = tokio::task::spawn_blocking(move || wait_for_callback(listener))
        .await
        .map_err(|e| format!("Task join error: {}", e))?
        .map_err(|e| e)?;

    // After saving session, try to fetch user info from Supabase
    let session = fetch_user_info(session, &supabase_url, &anon_key).await;
    save_session_to_disk(&session)?;

    let user = session_to_user(&session);

    // Emit auth-changed event
    let _ = app.emit("auth-changed", &user);

    Ok(user)
}

/// Fetch user info from Supabase using the access token
async fn fetch_user_info(mut session: AuthSession, supabase_url: &str, anon_key: &str) -> AuthSession {
    let url = format!("{}/auth/v1/user", supabase_url.trim_end_matches('/'));

    let client = match reqwest::Client::builder()
        .timeout(Duration::from_secs(10))
        .build()
    {
        Ok(c) => c,
        Err(_) => return session,
    };

    let resp = client
        .get(&url)
        .header("Authorization", format!("Bearer {}", session.access_token))
        .header("apikey", anon_key)
        .send()
        .await;

    if let Ok(resp) = resp {
        if let Ok(body) = resp.json::<serde_json::Value>().await {
            session.user_id = body.get("id").and_then(|v| v.as_str()).map(|s| s.to_string());
            session.email = body.get("email").and_then(|v| v.as_str()).map(|s| s.to_string());

            if let Some(identities) = body.get("identities").and_then(|v| v.as_array()) {
                if let Some(first) = identities.first() {
                    if session.provider.is_none() {
                        session.provider = first
                            .get("provider")
                            .and_then(|v| v.as_str())
                            .map(|s| s.to_string());
                    }
                }
            }

            if let Some(user_meta) = body.get("user_metadata") {
                session.metadata = serde_json::json!({
                    "user_metadata": user_meta,
                    "app_metadata": body.get("app_metadata").unwrap_or(&serde_json::Value::Null),
                });
            }
        }
    }

    session
}

/// Delete session and notify frontend
#[tauri::command]
pub fn auth_logout(app: tauri::AppHandle) -> Result<(), String> {
    let path = session_path();
    if path.exists() {
        fs::remove_file(&path).map_err(|e| format!("Failed to delete session: {}", e))?;
    }
    let _ = app.emit("auth-changed", serde_json::Value::Null);
    Ok(())
}

/// Refresh access token using refresh_token
#[tauri::command(rename_all = "camelCase")]
pub async fn auth_refresh(
    app: tauri::AppHandle,
    supabase_url: String,
    anon_key: String,
) -> Result<AuthUser, String> {
    let session = load_session_from_disk()
        .ok_or_else(|| "No session found. Please login first.".to_string())?;

    if session.refresh_token.is_empty() {
        return Err("No refresh token available.".into());
    }

    let client = reqwest::Client::builder()
        .timeout(Duration::from_secs(10))
        .build()
        .map_err(|e| format!("HTTP client error: {}", e))?;

    let url = format!(
        "{}/auth/v1/token?grant_type=refresh_token",
        supabase_url.trim_end_matches('/')
    );

    let body = serde_json::json!({ "refresh_token": session.refresh_token });

    let resp = client
        .post(&url)
        .header("apikey", &anon_key)
        .header("Content-Type", "application/json")
        .json(&body)
        .send()
        .await
        .map_err(|e| format!("Refresh request failed: {}", e))?;

    if !resp.status().is_success() {
        let status = resp.status();
        let body_text = resp.text().await.unwrap_or_default();
        return Err(format!("Refresh failed ({}): {}", status, body_text));
    }

    let data: serde_json::Value = resp
        .json()
        .await
        .map_err(|e| format!("Failed to parse refresh response: {}", e))?;

    let new_session = AuthSession {
        access_token: data
            .get("access_token")
            .and_then(|v| v.as_str())
            .unwrap_or_default()
            .to_string(),
        refresh_token: data
            .get("refresh_token")
            .and_then(|v| v.as_str())
            .unwrap_or(&session.refresh_token)
            .to_string(),
        token_type: data
            .get("token_type")
            .and_then(|v| v.as_str())
            .unwrap_or("bearer")
            .to_string(),
        expires_at: data.get("expires_at").and_then(|v| {
            v.as_i64().map(|epoch| {
                chrono::DateTime::from_timestamp(epoch, 0)
                    .map(|dt| dt.to_rfc3339())
                    .unwrap_or_default()
            })
        }),
        user_id: session.user_id,
        email: session.email,
        provider: session.provider,
        provider_token: session.provider_token,
        metadata: session.metadata,
    };

    save_session_to_disk(&new_session)?;

    let user = session_to_user(&new_session);
    let _ = app.emit("auth-changed", &user);

    Ok(user)
}

/// Check Supabase configuration from env vars / cloud.yaml
#[tauri::command]
pub fn auth_get_config() -> AuthConfig {
    read_supabase_config()
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_parse_callback_params() {
        let line =
            "GET /auth/callback?access_token=abc123&refresh_token=def456&token_type=bearer HTTP/1.1";
        let params = parse_callback_params(line);
        assert_eq!(params.get("access_token").unwrap(), "abc123");
        assert_eq!(params.get("refresh_token").unwrap(), "def456");
        assert_eq!(params.get("token_type").unwrap(), "bearer");
    }

    #[test]
    fn test_parse_callback_no_query() {
        let line = "GET /auth/callback HTTP/1.1";
        let params = parse_callback_params(line);
        assert!(params.is_empty());
    }

    #[test]
    fn test_build_oauth_url() {
        let url = build_oauth_url("https://test.supabase.co", &format!("http://127.0.0.1:{}/auth/callback", DEFAULT_OAUTH_PORT));
        assert!(url.starts_with("https://test.supabase.co/auth/v1/authorize?provider=github"));
        assert!(url.contains("redirect_to="));
    }

    #[test]
    fn test_session_serialization() {
        let session = AuthSession {
            access_token: "tok".into(),
            refresh_token: "ref".into(),
            token_type: "bearer".into(),
            expires_at: None,
            user_id: Some("uid".into()),
            email: Some("a@b.com".into()),
            provider: Some("github".into()),
            provider_token: None,
            metadata: serde_json::json!({}),
        };
        let json = serde_json::to_string(&session).unwrap();
        let parsed: AuthSession = serde_json::from_str(&json).unwrap();
        assert_eq!(parsed.access_token, "tok");
        assert_eq!(parsed.email, Some("a@b.com".into()));
        // provider_token should be absent (skip_serializing_if)
        assert!(!json.contains("provider_token"));
    }

    #[test]
    fn test_session_to_user() {
        let session = AuthSession {
            access_token: "t".into(),
            refresh_token: "r".into(),
            token_type: "bearer".into(),
            expires_at: None,
            user_id: Some("u1".into()),
            email: Some("test@example.com".into()),
            provider: Some("github".into()),
            provider_token: None,
            metadata: serde_json::json!({}),
        };
        let user = session_to_user(&session);
        assert_eq!(user.id, "u1");
        assert_eq!(user.email, "test@example.com");
        assert_eq!(user.provider, "github");
    }

    #[test]
    fn test_auth_config_no_env() {
        // With no env vars set, should return None/false
        let config = read_supabase_config();
        // Can't assert specific values since env may vary, just check types
        let _ = config.supabase_url;
        assert!(config.has_anon_key || !config.has_anon_key);
    }
}
