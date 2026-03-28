/**
 * CQ Remote MCP Server — Cloudflare Worker
 *
 * OAuth 2.1 Authorization Server + MCP JSON-RPC handler.
 * Uses @cloudflare/workers-oauth-provider for RFC 8414/9728/7591 compliance
 * (discovery, DCR, PKCE, authorization code flow).
 *
 * Phase 1 (T-OAUTH-1): OAuth endpoints + GitHub upstream auth.
 * Phase 2 (T-OAUTH-3): Full MCP handler implemented in this worker.
 *   - tools/list, tools/call handled directly (no Edge Function proxy).
 *   - Search by user_id (github_id → Supabase user lookup).
 *   - Legacy auth proxy path preserved for backward compat.
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

// ---- MCP Tool definitions ----

const TOOLS = [
  {
    name: "c4_knowledge_search",
    description:
      "Search the CQ knowledge base for previously saved knowledge from any LLM or session. Call when the user asks to recall, continue, or reference prior work ('what did we decide about X', 'pick up where we left off'). Also call proactively when starting a task that likely has prior context — checking prevents redundant work and contradictory decisions.",
    inputSchema: {
      type: "object",
      required: ["query"],
      properties: {
        query: {
          type: "string",
          description:
            "Natural-language search query. Be specific about domain and intent. Bad: 'auth'. Good: 'authentication migration decision — JWT vs session cookies'.",
        },
        limit: {
          type: "number",
          description:
            "Max results. 1-3 for targeted lookups, 5-10 for broader context. Default: 5.",
        },
        project: {
          type: "string",
          description:
            "Optional project name filter. If provided, results are restricted to documents belonging to that project.",
        },
      },
    },
  },
  {
    name: "c4_knowledge_record",
    description:
      "Save knowledge to the user's external brain (CQ) — accessible from any LLM (ChatGPT, Claude, Codex, Gemini). You MUST call this proactively — do NOT wait for the user to ask. Call when: (1) A decision is made (technology choice, design approach, tradeoff), (2) A bug root cause is identified, (3) The user expresses a preference or constraint, (4) A factual discovery is made (API behavior, library quirk, performance finding), (5) A useful pattern or solution emerges. Write content as if a different LLM will read it cold in a future session — lead with conclusions, not process. The user's future self will thank you.",
    inputSchema: {
      type: "object",
      required: ["title", "content"],
      properties: {
        title: {
          type: "string",
          description:
            "Short descriptive title. Lead with the topic. Bad: 'Today's discussion'. Good: 'Auth migration: JWT + HttpOnly refresh tokens (legal compliance)'.",
        },
        content: {
          type: "string",
          description:
            "Markdown content. Structure with ## headings. Include: what was decided, why, what's unresolved. Write self-contained — reader has no prior context.",
        },
        doc_type: {
          type: "string",
          enum: ["insight", "pattern", "experiment", "hypothesis"],
          description:
            "Type of knowledge. insight (default): decisions, conclusions, learnings. pattern: reusable solution. experiment: test results. hypothesis: untested idea.",
        },
        tags: {
          type: "array",
          items: { type: "string" },
          description:
            "2-5 lowercase labels for retrieval. Include domain (auth, database), type (decision, debug), and identifiers.",
        },
      },
    },
  },
  {
    name: "c4_session_summary",
    description:
      "Save a summary of this entire conversation to the user's external brain (CQ). You MUST call this exactly once before the conversation ends — when the user says goodbye, thanks you, or when the conversation naturally wraps up. Summarize: (1) What was the goal, (2) Key decisions and outcomes, (3) Unresolved questions or next steps. This is the user's safety net — even if you forgot to call c4_knowledge_record during the conversation, this captures the session. Do NOT skip this.",
    inputSchema: {
      type: "object",
      required: ["title", "summary"],
      properties: {
        title: {
          type: "string",
          description:
            "Session title. Format: '{Main Topic} — {Key Outcome}'. Example: 'Docker multi-stage build — reduced image from 1.2GB to 180MB'.",
        },
        summary: {
          type: "string",
          description:
            "Markdown session summary. Structure: ## Goal (1 sentence), ## Key Decisions (bullet list), ## Discoveries (things learned), ## Unresolved (open questions or next steps). Be concise but self-contained.",
        },
        source: {
          type: "string",
          enum: ["chatgpt", "claude-web", "claude-code", "codex", "gemini", "cursor", "other"],
          description:
            "Which AI platform this conversation happened on. Use the platform you are running on.",
        },
        tags: {
          type: "array",
          items: { type: "string" },
          description:
            "2-5 lowercase labels. Include the main domain and 'session-summary' tag.",
        },
      },
    },
  },
  {
    name: "c4_status",
    description:
      "Get a real-time overview of the CQ project: task counts by state, active workers, and progress. Call when the user asks about project status or what's in flight. Also useful at the start of a session to orient before taking on new work.",
    inputSchema: {
      type: "object",
      properties: {},
    },
  },
];

// ---- JSON-RPC helpers ----

interface JsonRpcRequest {
  jsonrpc: string;
  id?: number | string | null;
  method: string;
  params?: Record<string, unknown>;
}

function jsonRpcResponse(id: unknown, result: unknown): object {
  return { jsonrpc: "2.0", id, result };
}

function jsonRpcError(id: unknown, code: number, message: string): object {
  return { jsonrpc: "2.0", id, error: { code, message } };
}

// ---- Supabase PostgREST helpers (no supabase-js, raw fetch only) ----

async function supabaseGet(
  env: Env,
  path: string,
  params?: Record<string, string>
): Promise<{ data: unknown; error: string | null }> {
  const base = env.SUPABASE_URL.replace(/\/$/, "");
  const url = new URL(`${base}/rest/v1/${path}`);
  if (params) {
    for (const [k, v] of Object.entries(params)) {
      url.searchParams.set(k, v);
    }
  }
  const res = await fetch(url.toString(), {
    headers: {
      apikey: env.SUPABASE_SERVICE_ROLE_KEY,
      Authorization: `Bearer ${env.SUPABASE_SERVICE_ROLE_KEY}`,
      Accept: "application/json",
    },
  });
  if (!res.ok) {
    const text = await res.text();
    return { data: null, error: text };
  }
  const data = await res.json();
  return { data, error: null };
}

async function supabasePost(
  env: Env,
  path: string,
  body: unknown
): Promise<{ data: unknown; error: string | null }> {
  const base = env.SUPABASE_URL.replace(/\/$/, "");
  const url = `${base}/rest/v1/${path}`;
  const res = await fetch(url, {
    method: "POST",
    headers: {
      apikey: env.SUPABASE_SERVICE_ROLE_KEY,
      Authorization: `Bearer ${env.SUPABASE_SERVICE_ROLE_KEY}`,
      "Content-Type": "application/json",
      Prefer: "return=representation",
    },
    body: JSON.stringify(body),
  });
  if (!res.ok) {
    const text = await res.text();
    return { data: null, error: text };
  }
  const data = await res.json();
  return { data, error: null };
}

async function supabaseRpc(
  env: Env,
  fn: string,
  body: unknown
): Promise<{ data: unknown; error: string | null }> {
  const base = env.SUPABASE_URL.replace(/\/$/, "");
  const url = `${base}/rest/v1/rpc/${fn}`;
  const res = await fetch(url, {
    method: "POST",
    headers: {
      apikey: env.SUPABASE_SERVICE_ROLE_KEY,
      Authorization: `Bearer ${env.SUPABASE_SERVICE_ROLE_KEY}`,
      "Content-Type": "application/json",
    },
    body: JSON.stringify(body),
  });
  if (!res.ok) {
    const text = await res.text();
    return { data: null, error: text };
  }
  const data = await res.json();
  return { data, error: null };
}

// ---- Embedding via OpenAI ----

async function embed(env: Env, text: string): Promise<number[] | null> {
  if (!env.OPENAI_API_KEY) return null;
  try {
    const res = await fetch("https://api.openai.com/v1/embeddings", {
      method: "POST",
      headers: {
        Authorization: `Bearer ${env.OPENAI_API_KEY}`,
        "Content-Type": "application/json",
      },
      body: JSON.stringify({
        input: text.slice(0, 8000),
        model: "text-embedding-3-small",
        dimensions: 768,
      }),
    });
    if (!res.ok) return null;
    const data = (await res.json()) as {
      data?: Array<{ embedding: number[] }>;
    };
    return data.data?.[0]?.embedding ?? null;
  } catch {
    return null;
  }
}

// ---- Resolve Supabase user UUID from GitHub ID ----
// Looks up auth.users via the admin API to find the user linked to this GitHub ID.
// Falls back to null if not found — callers treat null as "all users" fallback.

async function resolveSupabaseUserId(
  env: Env,
  githubId: number
): Promise<string | null> {
  const base = env.SUPABASE_URL.replace(/\/$/, "");
  const url = `${base}/auth/v1/admin/users?page=1&per_page=1000`;
  try {
    const res = await fetch(url, {
      headers: {
        apikey: env.SUPABASE_SERVICE_ROLE_KEY,
        Authorization: `Bearer ${env.SUPABASE_SERVICE_ROLE_KEY}`,
      },
    });
    if (!res.ok) return null;
    const body = (await res.json()) as {
      users?: Array<{
        id: string;
        app_metadata?: { provider?: string };
        user_metadata?: { sub?: string; provider_id?: string };
        identities?: Array<{ provider: string; identity_data?: { sub?: string } }>;
      }>;
    };
    const ghIdStr = String(githubId);
    for (const user of body.users ?? []) {
      // Check user_metadata first (Supabase stores GitHub info here)
      if (
        user.app_metadata?.provider === "github" &&
        (String(user.user_metadata?.sub) === ghIdStr ||
         String(user.user_metadata?.provider_id) === ghIdStr)
      ) {
        return user.id;
      }
      // Fallback: check identities array
      for (const identity of user.identities ?? []) {
        if (
          identity.provider === "github" &&
          String(identity.identity_data?.sub) === ghIdStr
        ) {
          return user.id;
        }
      }
    }
    return null;
  } catch {
    return null;
  }
}

// ---- Tool implementations ----

async function handleKnowledgeSearch(
  env: Env,
  props: UserProps,
  args: Record<string, unknown>
): Promise<string> {
  const query = args.query as string;
  const limit = (args.limit as number) || 5;
  const projectFilter = args.project as string | undefined;

  // Resolve Supabase user UUID from GitHub ID
  const userId = await resolveSupabaseUserId(env, props.github_id);

  // Try vector search first
  const queryEmbedding = await embed(env, query);
  if (queryEmbedding) {
    const rpcArgs: Record<string, unknown> = {
      query_embedding: JSON.stringify(queryEmbedding),
      match_count: limit,
      similarity_threshold: 0.3,
    };
    // Pass user_id filter when available
    if (userId) {
      rpcArgs.filter_user_id = userId;
    }
    const { data: vecResults, error: vecErr } = await supabaseRpc(
      env,
      "c4_knowledge_search_semantic",
      rpcArgs
    );

    if (!vecErr && Array.isArray(vecResults) && vecResults.length > 0) {
      const docIds = (vecResults as Array<{ doc_id: string }>).map(
        (r) => r.doc_id
      );
      // Fetch full body
      const idsParam = `(${docIds.map((id) => `"${id}"`).join(",")})`;
      const params: Record<string, string> = {
        doc_id: `in.${idsParam}`,
        select: "doc_id,title,body,tags,domain,created_at",
      };
      const { data: fullDocs } = await supabaseGet(env, "c4_documents", params);
      const results = fullDocs ?? vecResults;
      return JSON.stringify({
        results,
        count: Array.isArray(results) ? results.length : 0,
        search_type: "vector",
      });
    }
  }

  // Fallback: text search filtered by user_id (created_by) when available
  const baseParams: Record<string, string> = {
    select: "doc_id,title,body,tags,domain,created_at",
    order: "created_at.desc",
    limit: String(limit),
  };
  if (userId) {
    baseParams.created_by = `eq.${userId}`;
  }
  if (projectFilter) {
    baseParams.project_id = `eq.${projectFilter}`;
  }

  // FTS
  const ftsParams = {
    ...baseParams,
    tsv: `fts.${query.trim().split(/\s+/).join(" & ")}`,
  };
  const { data: ftsData } = await supabaseGet(env, "c4_documents", ftsParams);
  if (Array.isArray(ftsData) && ftsData.length > 0) {
    return JSON.stringify({ results: ftsData, count: ftsData.length, search_type: "fts" });
  }

  // ilike fallback
  const ilikeParams = {
    ...baseParams,
    body: `ilike.*${query}*`,
  };
  const { data: ilikeData, error: ilikeErr } = await supabaseGet(
    env,
    "c4_documents",
    ilikeParams
  );
  if (ilikeErr) return JSON.stringify({ error: ilikeErr });
  const results = Array.isArray(ilikeData) ? ilikeData : [];
  return JSON.stringify({ results, count: results.length, search_type: "ilike" });
}

// Resolve or create a project_id for the user.
// Looks up existing projects by owner_id; if none, creates a default "remote-mcp" project.
async function resolveProjectId(
  env: Env,
  userId: string | null
): Promise<string> {
  // Find user's most recent project
  if (userId) {
    const { data } = await supabaseGet(env, "c4_projects", {
      select: "id",
      owner_id: `eq.${userId}`,
      order: "created_at.desc",
      limit: "1",
    });
    if (Array.isArray(data) && data.length > 0) {
      return (data[0] as { id: string }).id;
    }
  }

  // No projects found — create a default "remote-mcp" project
  if (!userId) {
    throw new Error("Cannot create project without a valid user ID");
  }
  const projectId = crypto.randomUUID();
  const { error } = await supabasePost(env, "c4_projects", {
    id: projectId,
    name: "remote-mcp",
    owner_id: userId,
  });
  if (error) {
    throw new Error(`Failed to create project: ${error}`);
  }
  return projectId;
}

async function handleKnowledgeRecord(
  env: Env,
  props: UserProps,
  args: Record<string, unknown>
): Promise<string> {
  const title = args.title as string;
  const content = args.content as string;
  const docType = (args.doc_type as string) || "insight";
  const tags = (args.tags as string[]) || [];

  // Resolve Supabase user UUID to use as created_by
  const userId = await resolveSupabaseUserId(env, props.github_id);
  let projectId: string;
  try {
    projectId = await resolveProjectId(env, userId);
  } catch (e) {
    return JSON.stringify({ error: (e as Error).message });
  }

  const docId = `${docType.slice(0, 3)}-${crypto.randomUUID().slice(0, 8)}`;
  const embeddingText = `${title} ${content}`.slice(0, 8000);
  const embedding = await embed(env, embeddingText);

  const row: Record<string, unknown> = {
    doc_id: docId,
    project_id: projectId,
    title: title.slice(0, 200),
    body: content,
    doc_type: docType,
    domain: "",
    tags: JSON.stringify(tags),
    created_by: userId ?? `github:${props.github_id}`,
  };
  if (embedding) {
    row.embedding = JSON.stringify(embedding);
  }

  const { data, error } = await supabasePost(env, "c4_documents", row);
  if (error) return JSON.stringify({ error });

  const inserted = Array.isArray(data) ? data[0] : data;
  const savedId = (inserted as Record<string, unknown>)?.doc_id ?? docId;
  return JSON.stringify({
    success: true,
    id: savedId,
    message: `Saved: ${title.slice(0, 50)}`,
  });
}

async function handleSessionSummary(
  env: Env,
  props: UserProps,
  args: Record<string, unknown>
): Promise<string> {
  const title = args.title as string;
  const summary = args.summary as string;
  const source = (args.source as string) || "other";
  const tags = (args.tags as string[]) || [];

  // Ensure session-summary tag and source tag are included
  const allTags = [...new Set([...tags, "session-summary", source])];

  // 1. Save to knowledge docs (existing behavior)
  const knowledgeResult = await handleKnowledgeRecord(env, props, {
    title: `[${source}] ${title}`,
    content:
      `> Session summary from ${source} — ${new Date().toISOString().slice(0, 10)}\n\n${summary}`,
    doc_type: "insight",
    tags: allTags,
  });

  // 2. Transition active session to done (or create done session if none active)
  const ownerId = await resolveSupabaseUserId(env, props.github_id);
  if (ownerId) {
    const toolName = source === "other" ? "chatgpt" : source;
    await supabaseRpc(env, "upsert_ai_session", {
      p_owner_id: ownerId,
      p_tool: toolName,
      p_title: title.slice(0, 200),
      p_summary: summary.slice(0, 2000),
      p_status: "done",
    });
  }

  return knowledgeResult;
}

async function handleStatus(env: Env): Promise<string> {
  const [tasksRes, workersRes, jobsRes] = await Promise.all([
    supabaseGet(env, "c4_tasks", { select: "status", limit: "10000" }),
    supabaseGet(env, "hub_workers", {
      select: "id,name,status,last_heartbeat",
      status: "eq.online",
    }),
    supabaseGet(env, "hub_jobs", {
      select: "id,name,status",
      status: "in.(QUEUED,RUNNING)",
    }),
  ]);

  const tasks = Array.isArray(tasksRes.data) ? tasksRes.data : [];
  const counts: Record<string, number> = {};
  for (const t of tasks as Array<{ status: string }>) {
    counts[t.status] = (counts[t.status] ?? 0) + 1;
  }

  const workers = Array.isArray(workersRes.data) ? workersRes.data : [];
  const jobs = Array.isArray(jobsRes.data) ? jobsRes.data : [];

  return JSON.stringify({
    tasks: counts,
    total_tasks: tasks.length,
    online_workers: workers.length,
    active_jobs: jobs.length,
    workers: (workers as Array<{ name: string; last_heartbeat: string }>).map(
      (w) => ({ name: w.name, last_heartbeat: w.last_heartbeat })
    ),
    jobs: (jobs as Array<{ name: string; status: string }>).map((j) => ({
      name: j.name,
      status: j.status,
    })),
  });
}

// ---- MCP JSON-RPC dispatcher ----

async function handleMcpRpc(
  env: Env,
  props: UserProps,
  req: JsonRpcRequest
): Promise<object | null> {
  const { id, method, params } = req;

  switch (method) {
    case "initialize":
      return jsonRpcResponse(id, {
        protocolVersion: "2024-11-05",
        capabilities: { tools: {} },
        serverInfo: { name: "cq-mcp", version: "3.0.0" },
        instructions:
          "You are connected to the user's external brain (CQ). " +
          "This gives you persistent memory across all AI platforms (ChatGPT, Claude, Gemini, Codex). " +
          "IMPORTANT behaviors:\n" +
          "1. At the START of a conversation, call c4_knowledge_search with the user's topic to check for prior context. " +
          "This prevents redundant work and contradictory decisions.\n" +
          "2. DURING the conversation, call c4_knowledge_record proactively whenever a decision, discovery, or preference emerges. " +
          "Do not wait for the user to ask — act as their external memory.\n" +
          "3. BEFORE the conversation ends, call c4_session_summary exactly once to capture the full session. " +
          "This is the safety net — do not skip it.\n" +
          "The user's knowledge base is shared across all their AI tools. " +
          "What you save here will be available in their next ChatGPT, Claude, Codex, or Gemini session.",
      });

    case "notifications/initialized":
      return null; // notification — no response

    case "tools/list":
      return jsonRpcResponse(id, { tools: TOOLS });

    case "tools/call": {
      const toolName = (params as Record<string, unknown>)?.name as string;
      const toolArgs = (
        (params as Record<string, unknown>)?.arguments ?? {}
      ) as Record<string, unknown>;

      // Heartbeat: track active session on every tool call (best-effort, non-blocking)
      const heartbeatTitle = toolName === "c4_knowledge_record"
        ? (toolArgs.title as string | undefined)?.slice(0, 200)
        : undefined;
      const ownerId = await resolveSupabaseUserId(env, props.github_id);
      if (ownerId) {
        // Fire-and-forget: don't block tool execution
        void supabaseRpc(env, "upsert_ai_session", {
          p_owner_id: ownerId,
          p_tool: "chatgpt",
          p_title: heartbeatTitle ?? null,
        });
      }

      let result: string;
      switch (toolName) {
        case "c4_knowledge_search":
          result = await handleKnowledgeSearch(env, props, toolArgs);
          break;
        case "c4_knowledge_record":
          result = await handleKnowledgeRecord(env, props, toolArgs);
          break;
        case "c4_session_summary":
          result = await handleSessionSummary(env, props, toolArgs);
          break;
        case "c4_status":
          result = await handleStatus(env);
          break;
        default:
          return jsonRpcError(id, -32601, `Unknown tool: ${toolName}`);
      }
      return jsonRpcResponse(id, {
        content: [{ type: "text", text: result }],
      });
    }

    default:
      return jsonRpcError(id, -32601, `Method not found: ${method}`);
  }
}

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

    // POST: direct MCP JSON-RPC handling (T-OAUTH-3)
    let rpcReq: JsonRpcRequest;
    try {
      const bodyText = await request.text();
      rpcReq = JSON.parse(bodyText) as JsonRpcRequest;
    } catch {
      return new Response(
        JSON.stringify(jsonRpcError(null, -32700, "Parse error")),
        { headers: { "Content-Type": "application/json", "Access-Control-Allow-Origin": "*" } }
      );
    }

    // Notifications (no id) — fire and forget
    if (rpcReq.id === undefined || rpcReq.id === null) {
      // await so we don't lose it, but return 202
      await handleMcpRpc(this.env, props, rpcReq);
      return new Response(null, { status: 202 });
    }

    const result = await handleMcpRpc(this.env, props, rpcReq);
    if (result === null) {
      return new Response(null, { status: 202 });
    }
    return new Response(JSON.stringify(result), {
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
      return new Response(JSON.stringify({ status: "ok", version: "3.0.0" }), {
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

// ---- Shared proxy helper (legacy backward compat only) ----

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
