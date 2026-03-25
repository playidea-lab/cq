// oauth-consent: Supabase Edge Function
// OAuth 2.1 consent screen for third-party app authorization (e.g., ChatGPT).
// GET  ?authorization_id=... → renders consent HTML
// POST ?authorization_id=... &decision=approve|deny → processes consent, redirects

const SUPABASE_URL = Deno.env.get("SUPABASE_URL") ?? "";
const SUPABASE_SERVICE_ROLE_KEY = Deno.env.get("SUPABASE_SERVICE_ROLE_KEY") ?? "";

interface AuthorizationDetails {
  id: string;
  client?: { name?: string };
  scopes?: string[];
  redirect_uri?: string;
}

async function getAuthorizationDetails(
  authorizationId: string,
): Promise<AuthorizationDetails | null> {
  const res = await fetch(
    `${SUPABASE_URL}/auth/v1/admin/oauth/authorizations/${authorizationId}`,
    {
      headers: {
        Authorization: `Bearer ${SUPABASE_SERVICE_ROLE_KEY}`,
        "Content-Type": "application/json",
        apikey: SUPABASE_SERVICE_ROLE_KEY,
      },
    },
  );
  if (!res.ok) return null;
  return res.json();
}

function renderConsentPage(details: AuthorizationDetails): string {
  const appName = details.client?.name ?? "Unknown App";
  const scopes = details.scopes ?? ["openid"];
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
      <ul>${scopeList}</ul>
    </div>
    <form method="POST" class="actions">
      <input type="hidden" name="authorization_id" value="${escapeHtml(details.id)}">
      <button type="submit" name="decision" value="deny" class="deny">Deny</button>
      <button type="submit" name="decision" value="approve" class="approve">Approve</button>
    </form>
  </div>
</body>
</html>`;
}

function escapeHtml(s: string): string {
  return s
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;")
    .replace(/'/g, "&#39;");
}

Deno.serve(async (req: Request) => {
  const url = new URL(req.url);

  // GET: render consent page
  if (req.method === "GET") {
    const authorizationId = url.searchParams.get("authorization_id");
    if (!authorizationId) {
      return new Response("Missing authorization_id", { status: 400 });
    }

    const details = await getAuthorizationDetails(authorizationId);
    if (!details) {
      return new Response("Invalid or expired authorization request", { status: 404 });
    }

    return new Response(renderConsentPage(details), {
      headers: { "Content-Type": "text/html; charset=utf-8" },
    });
  }

  // POST: process consent decision
  if (req.method === "POST") {
    let authorizationId: string | null = null;
    let decision: string | null = null;

    const contentType = req.headers.get("Content-Type") ?? "";
    if (contentType.includes("application/x-www-form-urlencoded")) {
      const formData = await req.formData();
      authorizationId = formData.get("authorization_id") as string;
      decision = formData.get("decision") as string;
    } else {
      const body = await req.json();
      authorizationId = body.authorization_id;
      decision = body.decision;
    }

    if (!authorizationId || !decision) {
      return new Response("Missing authorization_id or decision", { status: 400 });
    }

    if (decision !== "approve" && decision !== "deny") {
      return new Response("Invalid decision — must be 'approve' or 'deny'", { status: 400 });
    }

    // Call Supabase Auth to process the consent
    const res = await fetch(`${SUPABASE_URL}/auth/v1/oauth/authorize`, {
      method: "POST",
      headers: {
        Authorization: `Bearer ${SUPABASE_SERVICE_ROLE_KEY}`,
        "Content-Type": "application/json",
        apikey: SUPABASE_SERVICE_ROLE_KEY,
      },
      body: JSON.stringify({
        authorization_id: authorizationId,
        consent: decision,
      }),
    });

    if (!res.ok) {
      const err = await res.text();
      return new Response(`Authorization failed: ${err}`, { status: res.status });
    }

    const result = await res.json();

    // If the response contains a redirect_uri, redirect the user
    if (result.redirect_uri) {
      return new Response(null, {
        status: 302,
        headers: { Location: result.redirect_uri },
      });
    }

    // Fallback: show result
    return new Response(
      JSON.stringify({ status: decision === "approve" ? "approved" : "denied" }),
      { headers: { "Content-Type": "application/json" } },
    );
  }

  return new Response("Method not allowed", { status: 405 });
});
