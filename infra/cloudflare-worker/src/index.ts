/**
 * CQ Remote MCP Server — Cloudflare Worker
 *
 * OAuth 2.1 Authorization Server + MCP JSON-RPC handler.
 * Uses @cloudflare/workers-oauth-provider for RFC 8414/9728/7591 compliance
 * (discovery, DCR, PKCE, authorization code flow).
 *
 * Phase 1 (T-OAUTH-1): OAuth endpoints + GitHub upstream auth.
 *   MCP JSON-RPC proxied to Supabase Edge Function for backward compat.
 * Phase 2 (T-OAUTH-3): Full MCP handler implemented in this worker.
 * Phase 3: Edge Function retired.
 */

import OAuthProvider, {
  type OAuthHelpers,
  type AuthRequest,
} from "@cloudflare/workers-oauth-provider";
import { WorkerEntrypoint } from "cloudflare:workers";

// ---- Environment bindings ----

export interface Env {
  // KV namespace for OAuth state (GitHub state, supplemental data)
  // Binding name must match [[kv_namespaces]] binding in wrangler.toml
  OAUTH_KV: KVNamespace;
  // Injected by OAuthProvider framework as OAuthHelpers
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  OAUTH_PROVIDER: any;
  // GitHub OAuth App credentials (set via `wrangler secret put`)
  GITHUB_CLIENT_ID: string;
  GITHUB_CLIENT_SECRET: string;
  // Supabase backend
  SUPABASE_URL: string;
  SUPABASE_SERVICE_ROLE_KEY: string;
  // OpenAI embeddings (optional)
  OPENAI_API_KEY?: string;
  // Legacy API key for backward-compat auth (optional)
  MCP_API_KEY?: string;
}

// Props stored in the OAuth token, passed to the API handler via ctx.props
interface UserProps {
  github_id: number;
  github_login: string;
}

const MCP_SERVER_URL = "https://mcp.pilab.kr";

// ---- API handler: receives authenticated MCP requests ----

class McpApiHandler extends WorkerEntrypoint<Env> {
  async fetch(request: Request): Promise<Response> {
    // ctx.props contains the UserProps stored in the OAuth token
    const props = this.ctx.props as UserProps;

    // CORS preflight
    if (request.method === "OPTIONS") {
      return new Response(null, {
        headers: {
          "Access-Control-Allow-Origin": "*",
          "Access-Control-Allow-Methods": "GET, POST, OPTIONS",
          "Access-Control-Allow-Headers": "Content-Type, Authorization",
        },
      });
    }

    // GET: SSE keepalive (Streamable HTTP spec)
    if (request.method === "GET") {
      const body = new ReadableStream({
        start(controller) {
          const encoder = new TextEncoder();
          const interval = setInterval(() => {
            controller.enqueue(encoder.encode(": keepalive\n\n"));
          }, 15000);
          request.signal.addEventListener("abort", () => {
            clearInterval(interval);
            controller.close();
          });
        },
      });
      return new Response(body, {
        headers: {
          "Content-Type": "text/event-stream",
          "Cache-Control": "no-cache",
          "Access-Control-Allow-Origin": "*",
        },
      });
    }

    if (request.method !== "POST") {
      return new Response("Method not allowed", { status: 405 });
    }

    // POST: JSON-RPC
    // Phase 1: proxy to Supabase Edge Function, passing authenticated user identity.
    // T-OAUTH-3 will replace this with a full in-worker MCP handler.
    const supabaseUrl = this.env.SUPABASE_URL.replace(/\/$/, "");
    const upstreamUrl = `${supabaseUrl}/functions/v1/mcp-server`;

    const bodyText = await request.text();

    const upstreamRes = await fetch(upstreamUrl, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        Authorization: `Bearer ${this.env.SUPABASE_SERVICE_ROLE_KEY}`,
        // Pass authenticated user identity to upstream Edge Function
        "X-MCP-Github-Id": String(props.github_id ?? ""),
        "X-MCP-Github-Login": String(props.github_login ?? ""),
      },
      body: bodyText,
    });

    const responseBody = await upstreamRes.text();
    return new Response(responseBody, {
      status: upstreamRes.status,
      headers: {
        "Content-Type": "application/json",
        "Access-Control-Allow-Origin": "*",
      },
    });
  }
}

// ---- Default handler: OAuth UI routes + GitHub callback + legacy auth ----

const defaultHandler = {
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  async fetch(request: Request, env: any, _ctx: ExecutionContext): Promise<Response> {
    const url = new URL(request.url);
    // env.OAUTH_PROVIDER is injected by the framework as OAuthHelpers
    const oauthHelpers = env.OAUTH_PROVIDER as OAuthHelpers;

    // Health check
    if (url.pathname === "/health") {
      return new Response(JSON.stringify({ status: "ok", version: "1.0.0" }), {
        headers: { "Content-Type": "application/json" },
      });
    }

    // /authorize — redirect user to GitHub OAuth
    if (url.pathname === "/authorize") {
      return handleAuthorize(request, env, oauthHelpers);
    }

    // /callback — receive GitHub OAuth callback, issue authorization code
    if (url.pathname === "/callback") {
      return handleCallback(request, env, oauthHelpers);
    }

    // Legacy backward-compat: URL token or API key passes through to Edge Function
    // without OAuth. Supports existing integrations that predate OAuth 2.1.
    if (url.pathname === "/mcp" || url.pathname.startsWith("/mcp/")) {
      const urlToken = url.searchParams.get("token");
      const apiKey = request.headers.get("X-API-Key");
      const legacyKey = env.MCP_API_KEY;

      if (legacyKey && (urlToken === legacyKey || apiKey === legacyKey)) {
        return proxyToEdgeFunction(request, env, { legacyAuth: true });
      }
    }

    return new Response("Not found", { status: 404 });
  },
};

// ---- GitHub OAuth upstream helpers ----

async function handleAuthorize(
  request: Request,
  env: Env,
  oauthHelpers: OAuthHelpers
): Promise<Response> {
  // Parse and validate the OAuth authorization request (client_id, redirect_uri, PKCE, etc.)
  const oauthReq: AuthRequest = await oauthHelpers.parseAuthRequest(request);

  // Store the parsed OAuth request keyed by a random state value
  const state = crypto.randomUUID();
  await env.OAUTH_KV.put(
    `github_state:${state}`,
    JSON.stringify(oauthReq),
    { expirationTtl: 600 } // 10 minutes
  );

  // Redirect user to GitHub for authentication
  const params = new URLSearchParams({
    client_id: env.GITHUB_CLIENT_ID,
    redirect_uri: `${MCP_SERVER_URL}/callback`,
    scope: "read:user user:email",
    state,
  });

  return Response.redirect(
    `https://github.com/login/oauth/authorize?${params}`,
    302
  );
}

async function handleCallback(
  request: Request,
  env: Env,
  oauthHelpers: OAuthHelpers
): Promise<Response> {
  const url = new URL(request.url);
  const code = url.searchParams.get("code");
  const state = url.searchParams.get("state");

  if (!code || !state) {
    return new Response("Missing code or state", { status: 400 });
  }

  // Retrieve the original OAuth request
  const oauthReqJson = await env.OAUTH_KV.get(`github_state:${state}`);
  if (!oauthReqJson) {
    return new Response("State expired or invalid — please try again", {
      status: 400,
    });
  }
  const oauthReq: AuthRequest = JSON.parse(oauthReqJson);
  await env.OAUTH_KV.delete(`github_state:${state}`);

  // Exchange GitHub code for access token
  const tokenRes = await fetch("https://github.com/login/oauth/access_token", {
    method: "POST",
    headers: {
      Accept: "application/json",
      "Content-Type": "application/json",
    },
    body: JSON.stringify({
      client_id: env.GITHUB_CLIENT_ID,
      client_secret: env.GITHUB_CLIENT_SECRET,
      code,
      redirect_uri: `${MCP_SERVER_URL}/callback`,
    }),
  });

  if (!tokenRes.ok) {
    return new Response("GitHub token exchange failed", { status: 502 });
  }

  const tokenData = (await tokenRes.json()) as {
    access_token?: string;
    error?: string;
    error_description?: string;
  };
  if (!tokenData.access_token) {
    return new Response(
      `GitHub error: ${tokenData.error_description ?? tokenData.error ?? "unknown"}`,
      { status: 502 }
    );
  }

  // Fetch GitHub user profile
  const userRes = await fetch("https://api.github.com/user", {
    headers: {
      Authorization: `Bearer ${tokenData.access_token}`,
      Accept: "application/vnd.github.v3+json",
      "User-Agent": "cq-mcp-worker/1.0",
    },
  });

  if (!userRes.ok) {
    return new Response("Failed to fetch GitHub user profile", { status: 502 });
  }

  const ghUser = (await userRes.json()) as {
    id: number;
    login: string;
    email?: string | null;
    name?: string | null;
  };

  // Issue OAuth authorization code and redirect back to the MCP client
  const { redirectTo } = await oauthHelpers.completeAuthorization({
    request: oauthReq,
    userId: String(ghUser.id),
    metadata: {
      label: `${ghUser.login} via GitHub`,
      github_login: ghUser.login,
      github_email: ghUser.email ?? null,
    },
    scope: oauthReq.scope,
    // Props are encrypted into the token and passed to McpApiHandler.ctx.props
    props: {
      github_id: ghUser.id,
      github_login: ghUser.login,
    } satisfies UserProps,
  });

  return Response.redirect(redirectTo, 302);
}

// ---- Shared proxy helper ----

async function proxyToEdgeFunction(
  request: Request,
  env: Env,
  options: { legacyAuth: boolean }
): Promise<Response> {
  const supabaseUrl = env.SUPABASE_URL.replace(/\/$/, "");
  const upstreamUrl = `${supabaseUrl}/functions/v1/mcp-server`;
  const bodyText = request.method === "POST" ? await request.text() : undefined;

  const upstreamRes = await fetch(upstreamUrl, {
    method: request.method,
    headers: {
      "Content-Type": "application/json",
      Authorization: `Bearer ${env.SUPABASE_SERVICE_ROLE_KEY}`,
      ...(options.legacyAuth ? { "X-Legacy-Auth": "true" } : {}),
    },
    body: bodyText,
  });

  const responseBody = await upstreamRes.text();
  return new Response(responseBody, {
    status: upstreamRes.status,
    headers: {
      "Content-Type": "application/json",
      "Access-Control-Allow-Origin": "*",
    },
  });
}

// ---- Worker export ----

export default new OAuthProvider({
  // All requests to /mcp require a valid OAuth access token
  apiRoute: "/mcp",
  apiHandler: McpApiHandler,
  defaultHandler,
  // OAuth endpoints
  // Framework auto-registers:
  //   GET  /.well-known/oauth-authorization-server  (RFC 8414)
  //   GET  /.well-known/oauth-protected-resource    (RFC 9728)
  //   POST /register                                (RFC 7591 DCR)
  //   POST /token                                   (token + refresh)
  authorizeEndpoint: "/authorize",
  tokenEndpoint: "/token",
  clientRegistrationEndpoint: "/register",
  // Scopes available for MCP clients to request
  scopesSupported: ["mcp"],
  // Token TTLs
  accessTokenTTL: 3600,      // 1 hour
});
