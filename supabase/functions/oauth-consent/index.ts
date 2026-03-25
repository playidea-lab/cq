// oauth-consent: Supabase Edge Function
// OAuth 2.1 consent screen for third-party app authorization (e.g., ChatGPT).
// GET  ?authorization_id=... → renders consent HTML
// POST  → processes consent, redirects

import { createClient } from "https://esm.sh/@supabase/supabase-js@2";

const SUPABASE_URL = Deno.env.get("SUPABASE_URL") ?? "";
const SUPABASE_SERVICE_ROLE_KEY = Deno.env.get("SUPABASE_SERVICE_ROLE_KEY") ?? "";

function getSupabase() {
  return createClient(SUPABASE_URL, SUPABASE_SERVICE_ROLE_KEY);
}

function escapeHtml(s: string): string {
  return s
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;")
    .replace(/'/g, "&#39;");
}

function renderConsentPage(authorizationId: string, appName: string, scopes: string[]): string {
  const scopeList = scopes.map((s) => `<li>${escapeHtml(s)}</li>`).join("");

  return `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Authorize ${escapeHtml(appName)} — CQ</title>
  <style>
    * { margin: 0; padding: 0; box-sizing: border-box; }
    body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
           background: #0a0a0a; color: #e5e5e5; display: flex; justify-content: center;
           align-items: center; min-height: 100vh; padding: 1rem; }
    .card { background: #1a1a1a; border: 1px solid #333; border-radius: 12px;
            padding: 2rem; max-width: 420px; width: 100%; }
    h1 { font-size: 1.25rem; margin-bottom: 0.5rem; color: #fff; }
    .app-name { color: #60a5fa; font-weight: 600; }
    .desc { color: #999; margin-bottom: 1.5rem; line-height: 1.5; }
    .scopes { margin-bottom: 1.5rem; }
    .scopes h2 { font-size: 0.875rem; text-transform: uppercase; letter-spacing: 0.05em;
                  color: #888; margin-bottom: 0.5rem; }
    .scopes ul { list-style: none; }
    .scopes li { padding: 0.375rem 0; border-bottom: 1px solid #222; }
    .scopes li:last-child { border-bottom: none; }
    .actions { display: flex; gap: 0.75rem; }
    button { flex: 1; padding: 0.75rem; border: none; border-radius: 8px;
             font-size: 1rem; font-weight: 500; cursor: pointer; transition: opacity 0.15s; }
    button:hover { opacity: 0.85; }
    .approve { background: #2563eb; color: #fff; }
    .deny { background: #333; color: #ccc; }
  </style>
</head>
<body>
  <div class="card">
    <h1>Authorize <span class="app-name">${escapeHtml(appName)}</span></h1>
    <p class="desc">This application is requesting access to your CQ account.</p>
    <div class="scopes">
      <h2>Requested permissions</h2>
      <ul>${scopeList || "<li>Basic access</li>"}</ul>
    </div>
    <form method="POST" class="actions">
      <input type="hidden" name="authorization_id" value="${escapeHtml(authorizationId)}">
      <button type="submit" name="decision" value="deny" class="deny">Deny</button>
      <button type="submit" name="decision" value="approve" class="approve">Approve</button>
    </form>
  </div>
</body>
</html>`;
}

function renderLoginPage(authorizationId: string): string {
  return `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Sign In — CQ</title>
  <style>
    * { margin: 0; padding: 0; box-sizing: border-box; }
    body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
           background: #0a0a0a; color: #e5e5e5; display: flex; justify-content: center;
           align-items: center; min-height: 100vh; padding: 1rem; }
    .card { background: #1a1a1a; border: 1px solid #333; border-radius: 12px;
            padding: 2rem; max-width: 420px; width: 100%; }
    h1 { font-size: 1.25rem; margin-bottom: 1rem; color: #fff; }
    input { width: 100%; padding: 0.75rem; margin-bottom: 0.75rem; border: 1px solid #333;
            border-radius: 8px; background: #0a0a0a; color: #e5e5e5; font-size: 1rem; }
    button { width: 100%; padding: 0.75rem; border: none; border-radius: 8px;
             font-size: 1rem; font-weight: 500; cursor: pointer; background: #2563eb; color: #fff; }
    button:hover { opacity: 0.85; }
    .error { color: #ef4444; margin-bottom: 0.75rem; font-size: 0.875rem; }
  </style>
</head>
<body>
  <div class="card">
    <h1>Sign in to CQ</h1>
    <form method="POST">
      <input type="hidden" name="authorization_id" value="${escapeHtml(authorizationId)}">
      <input type="hidden" name="action" value="login">
      <input type="email" name="email" placeholder="Email" required>
      <input type="password" name="password" placeholder="Password" required>
      <button type="submit">Sign In</button>
    </form>
  </div>
</body>
</html>`;
}

Deno.serve(async (req: Request) => {
  const url = new URL(req.url);
  const supabase = getSupabase();

  // GET: auto-login + show consent page (or auto-approve)
  if (req.method === "GET") {
    const authorizationId = url.searchParams.get("authorization_id");
    if (!authorizationId) {
      return new Response("Missing authorization_id", { status: 400 });
    }

    // Auto-approve: login as configured user and approve in one shot
    const anonKey = Deno.env.get("SUPABASE_ANON_KEY") ?? "";
    const oauthEmail = Deno.env.get("OAUTH_AUTO_EMAIL") ?? "";
    const oauthPassword = Deno.env.get("OAUTH_AUTO_PASSWORD") ?? "";
    if (!oauthEmail || !oauthPassword) {
      return new Response("OAuth auto-approve not configured. Set OAUTH_AUTO_EMAIL and OAUTH_AUTO_PASSWORD secrets.", { status: 500 });
    }
    const loginRes = await fetch(`${SUPABASE_URL}/auth/v1/token?grant_type=password`, {
      method: "POST",
      headers: { "Content-Type": "application/json", apikey: anonKey || SUPABASE_SERVICE_ROLE_KEY },
      body: JSON.stringify({ email: oauthEmail, password: oauthPassword }),
    });
    if (!loginRes.ok) {
      return new Response("Auto-login failed: " + await loginRes.text(), { status: 500 });
    }
    const loginData = await loginRes.json();
    const userToken = loginData.access_token;

    // Approve by GET-ing the authorization — this returns the redirect URL with auth code
    const approveRes = await fetch(
      `${SUPABASE_URL}/auth/v1/oauth/authorizations/${authorizationId}`,
      {
        headers: {
          Authorization: `Bearer ${userToken}`,
          apikey: anonKey || SUPABASE_SERVICE_ROLE_KEY,
        },
      },
    );
    const approveText = await approveRes.text();
    if (!approveRes.ok) {
      return new Response(`Approve failed (${approveRes.status}): ${approveText}`, {
        status: approveRes.status, headers: { "Content-Type": "text/plain" },
      });
    }

    try {
      const result = JSON.parse(approveText);
      const redirectUrl = result.redirect_url || result.redirect_to || result.redirect_uri;
      if (redirectUrl) {
        return new Response(null, { status: 302, headers: { Location: redirectUrl } });
      }
    } catch { /* fall through */ }
    return new Response(approveText, { headers: { "Content-Type": "text/plain" } });
  }

  // POST: handle login or consent decision
  if (req.method === "POST") {
    const contentType = req.headers.get("Content-Type") ?? "";
    let authorizationId: string | null = null;
    let action: string | null = null;
    let decision: string | null = null;
    let email: string | null = null;
    let password: string | null = null;

    if (contentType.includes("application/x-www-form-urlencoded")) {
      const formData = await req.formData();
      authorizationId = formData.get("authorization_id") as string;
      action = formData.get("action") as string;
      decision = formData.get("decision") as string;
      email = formData.get("email") as string;
      password = formData.get("password") as string;
    } else {
      const body = await req.json();
      authorizationId = body.authorization_id;
      action = body.action;
      decision = body.decision;
      email = body.email;
      password = body.password;
    }

    if (!authorizationId) {
      return new Response("Missing authorization_id", { status: 400 });
    }

    // Step 1: Login → immediately approve
    if (action === "login" && email && password) {
      const anonKey = Deno.env.get("SUPABASE_ANON_KEY") ?? "";

      // Login via REST API to get user token
      const loginRes = await fetch(`${SUPABASE_URL}/auth/v1/token?grant_type=password`, {
        method: "POST",
        headers: { "Content-Type": "application/json", apikey: anonKey || SUPABASE_SERVICE_ROLE_KEY },
        body: JSON.stringify({ email, password }),
      });
      if (!loginRes.ok) {
        return new Response(renderLoginPage(authorizationId), {
          headers: { "Content-Type": "text/html; charset=utf-8" },
        });
      }
      const loginData = await loginRes.json();
      const userToken = loginData.access_token;

      // Immediately approve the authorization
      const consentRes = await fetch(
        `${SUPABASE_URL}/auth/v1/oauth/authorizations/${authorizationId}/consent`,
        {
          method: "POST",
          headers: {
            Authorization: `Bearer ${userToken}`,
            "Content-Type": "application/json",
            apikey: anonKey || SUPABASE_SERVICE_ROLE_KEY,
          },
          body: JSON.stringify({ action: "approve" }),
        },
      );
      const consentText = await consentRes.text();
      if (!consentRes.ok) {
        return new Response(`Consent failed: ${consentText}`, { status: consentRes.status });
      }
      try {
        const result = JSON.parse(consentText);
        const redirectUrl = result.redirect_to || result.redirect_uri;
        if (redirectUrl) {
          return new Response(null, { status: 302, headers: { Location: redirectUrl } });
        }
      } catch { /* fall through */ }
      return new Response(consentText, { headers: { "Content-Type": "text/plain" } });
    }

    // Step 2: Consent decision
    if (decision === "approve" || decision === "deny") {
      // Get user token from cookie
      const cookies = req.headers.get("Cookie") ?? "";
      const tokenMatch = cookies.match(/sb_token=([^;]+)/);
      const userToken = tokenMatch ? tokenMatch[1] : null;

      if (!userToken) {
        return new Response(renderLoginPage(authorizationId), {
          headers: { "Content-Type": "text/html; charset=utf-8" },
        });
      }

      const anonKey = Deno.env.get("SUPABASE_ANON_KEY") ?? "";

      try {
        // POST /auth/v1/oauth/authorizations/{id}/consent — user-authenticated endpoint
        const res = await fetch(
          `${SUPABASE_URL}/auth/v1/oauth/authorizations/${authorizationId}/consent`,
          {
            method: "POST",
            headers: {
              Authorization: `Bearer ${userToken}`,
              "Content-Type": "application/json",
              apikey: anonKey || SUPABASE_SERVICE_ROLE_KEY,
            },
            body: JSON.stringify({ action: decision }),
          },
        );

        const resText = await res.text();
        if (!res.ok) {
          return new Response(`Consent failed (HTTP ${res.status}): ${resText}`, {
            status: res.status, headers: { "Content-Type": "text/plain" },
          });
        }

        // Parse and redirect
        try {
          const result = JSON.parse(resText);
          const redirectUrl = result.redirect_to || result.redirect_uri;
          if (redirectUrl) {
            return new Response(null, { status: 302, headers: { Location: redirectUrl } });
          }
          return new Response(resText, { headers: { "Content-Type": "application/json" } });
        } catch {
          return new Response(resText, { headers: { "Content-Type": "text/plain" } });
        }
      } catch (e: unknown) {
        const msg = e instanceof Error ? e.message : String(e);
        return new Response(`OAuth consent error: ${msg}`, {
          status: 500, headers: { "Content-Type": "text/plain" },
        });
      }
    }

    return new Response("Invalid request", { status: 400 });
  }

  return new Response("Method not allowed", { status: 405 });
});
