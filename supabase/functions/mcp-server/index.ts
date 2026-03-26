// mcp-server: Supabase Edge Function
// Streamable HTTP MCP server for ChatGPT/Claude remote connections.
// Exposes CQ knowledge tools (snapshot, recall, status) via JSON-RPC 2.0.

import { createClient } from "https://esm.sh/@supabase/supabase-js@2";

const SUPABASE_URL = Deno.env.get("SUPABASE_URL") ?? "";
const SUPABASE_SERVICE_ROLE_KEY = Deno.env.get("SUPABASE_SERVICE_ROLE_KEY") ?? "";
const MCP_API_KEY = Deno.env.get("MCP_API_KEY") ?? "";
const OPENAI_API_KEY = Deno.env.get("OPENAI_API_KEY") ?? "";

function getSupabase() {
  return createClient(SUPABASE_URL, SUPABASE_SERVICE_ROLE_KEY);
}

// Generate 768-dim embedding via OpenAI text-embedding-3-small
async function embed(text: string): Promise<number[] | null> {
  if (!OPENAI_API_KEY) return null;
  try {
    const res = await fetch("https://api.openai.com/v1/embeddings", {
      method: "POST",
      headers: {
        Authorization: `Bearer ${OPENAI_API_KEY}`,
        "Content-Type": "application/json",
      },
      body: JSON.stringify({
        input: text.slice(0, 8000),
        model: "text-embedding-3-small",
        dimensions: 768,
      }),
    });
    if (!res.ok) return null;
    const data = await res.json();
    return data.data?.[0]?.embedding ?? null;
  } catch {
    return null;
  }
}

// --- Tool definitions ---

const TOOLS = [
  {
    name: "cq_knowledge_record",
    description: "Save knowledge to the CQ shared knowledge base — an external memory accessible from any LLM (ChatGPT, Claude, Codex, Cursor). Call this when the user asks to save, remember, or preserve something, OR when a conversation reaches a meaningful conclusion worth keeping. The knowledge becomes searchable by any LLM via c4_knowledge_search. Write the content as if a different person or LLM will read it cold in a future session — lead with conclusions, not process.",
    inputSchema: {
      type: "object",
      required: ["title", "content"],
      properties: {
        title: {
          type: "string",
          description: "Short descriptive title. Lead with the topic. Bad: 'Today's discussion'. Good: 'Auth migration: JWT + HttpOnly refresh tokens (legal compliance)'.",
        },
        content: {
          type: "string",
          description: "Markdown content. Structure with ## headings. Include: what was decided, why, what's unresolved. Write self-contained — reader has no prior context.",
        },
        doc_type: {
          type: "string",
          enum: ["insight", "pattern", "experiment", "hypothesis"],
          description: "Type of knowledge. insight (default): decisions, conclusions, learnings. pattern: reusable solution. experiment: test results. hypothesis: untested idea.",
        },
        tags: {
          type: "array",
          items: { type: "string" },
          description: "2-5 lowercase labels for retrieval. Include domain (auth, database), type (decision, debug), and identifiers.",
        },
      },
    },
  },
  {
    name: "cq_knowledge_search",
    description: "Search the CQ knowledge base for previously saved knowledge from any LLM or session. Call when the user asks to recall, continue, or reference prior work ('what did we decide about X', 'pick up where we left off'). Also call proactively when starting a task that likely has prior context — checking prevents redundant work and contradictory decisions.",
    inputSchema: {
      type: "object",
      required: ["query"],
      properties: {
        query: {
          type: "string",
          description: "Natural-language search query. Be specific about domain and intent. Bad: 'auth'. Good: 'authentication migration decision — JWT vs session cookies'.",
        },
        limit: {
          type: "number",
          description: "Max results. 1-3 for targeted lookups, 5-10 for broader context. Default: 5.",
        },
      },
    },
  },
  {
    name: "cq_status",
    description: "Get a real-time overview of the CQ project: task counts by state, active workers, and progress. Call when the user asks about project status or what's in flight. Also useful at the start of a session to orient before taking on new work.",
    inputSchema: {
      type: "object",
      properties: {},
    },
  },
];

// --- Tool implementations ---

async function handleKnowledgeRecord(args: Record<string, unknown>): Promise<string> {
  const supabase = getSupabase();
  const title = args.title as string;
  const content = args.content as string;
  const docType = (args.doc_type as string) || "insight";
  const tags = (args.tags as string[]) || [];

  const docId = `${docType.slice(0, 3)}-${crypto.randomUUID().slice(0, 8)}`;
  const embeddingText = `${title} ${content}`.slice(0, 8000);
  const embedding = await embed(embeddingText);

  const row: Record<string, unknown> = {
    doc_id: docId,
    project_id: Deno.env.get("C4_PROJECT_ID") ?? "00000000-0000-0000-0000-000000000000",
    title: title.slice(0, 200),
    body: content,
    doc_type: docType,
    domain: "",
    tags: JSON.stringify(tags),
    created_by: "remote-mcp",
  };
  if (embedding) {
    row.embedding = JSON.stringify(embedding);
  }

  const { data, error } = await supabase
    .from("c4_documents")
    .insert(row)
    .select("doc_id")
    .single();

  if (error) return JSON.stringify({ error: error.message });
  return JSON.stringify({ success: true, id: data.doc_id, message: `Saved: ${title.slice(0, 50)}` });
}

async function handleRecall(args: Record<string, unknown>): Promise<string> {
  const supabase = getSupabase();
  const query = args.query as string;
  const limit = (args.limit as number) || 5;

  const projectId = Deno.env.get("C4_PROJECT_ID") ?? "00000000-0000-0000-0000-000000000000";

  // Try vector search first (semantic similarity)
  const queryEmbedding = await embed(query);
  if (queryEmbedding) {
    const { data: vecResults, error: vecErr } = await supabase.rpc(
      "c4_knowledge_search_semantic",
      {
        query_embedding: JSON.stringify(queryEmbedding),
        match_count: limit,
        similarity_threshold: 0.3,
        filter_project_id: projectId,
      },
    );

    if (!vecErr && vecResults && vecResults.length > 0) {
      // Fetch full body for top results
      const docIds = vecResults.map((r: { doc_id: string }) => r.doc_id);
      const { data: fullDocs } = await supabase
        .from("c4_documents")
        .select("doc_id, title, body, tags, domain, created_at")
        .in("doc_id", docIds);

      return JSON.stringify({
        results: fullDocs || vecResults,
        count: (fullDocs || vecResults).length,
        search_type: "vector",
      });
    }
  }

  // Fallback: FTS
  const { data, error } = await supabase
    .from("c4_documents")
    .select("doc_id, title, body, tags, domain, created_at")
    .eq("project_id", projectId)
    .textSearch("tsv", query.split(" ").join(" & "))
    .order("created_at", { ascending: false })
    .limit(limit);

  if (!error && data && data.length > 0) {
    return JSON.stringify({ results: data, count: data.length, search_type: "fts" });
  }

  // Fallback: ilike
  const { data: fallback, error: fbErr } = await supabase
    .from("c4_documents")
    .select("doc_id, title, body, tags, domain, created_at")
    .eq("project_id", projectId)
    .ilike("body", `%${query}%`)
    .order("created_at", { ascending: false })
    .limit(limit);

  if (fbErr) return JSON.stringify({ error: fbErr.message });
  return JSON.stringify({ results: fallback || [], count: fallback?.length || 0, search_type: "ilike" });
}

async function handleStatus(): Promise<string> {
  const supabase = getSupabase();

  const { data: tasks } = await supabase
    .from("c4_tasks")
    .select("status")
    .limit(10000);

  const counts: Record<string, number> = {};
  for (const t of tasks || []) {
    counts[t.status] = (counts[t.status] || 0) + 1;
  }

  const { data: workers } = await supabase
    .from("hub_workers")
    .select("id, name, status, last_heartbeat")
    .eq("status", "online");

  const { data: jobs } = await supabase
    .from("hub_jobs")
    .select("id, name, status")
    .in("status", ["QUEUED", "RUNNING"]);

  return JSON.stringify({
    tasks: counts,
    total_tasks: tasks?.length || 0,
    online_workers: workers?.length || 0,
    active_jobs: jobs?.length || 0,
    workers: (workers || []).map(w => ({ name: w.name, last_heartbeat: w.last_heartbeat })),
    jobs: (jobs || []).map(j => ({ name: j.name, status: j.status })),
  });
}

// --- JSON-RPC 2.0 handler ---

interface JsonRpcRequest {
  jsonrpc: string;
  id?: number | string | null;
  method: string;
  params?: Record<string, unknown>;
}

function jsonRpcResponse(id: unknown, result: unknown) {
  return { jsonrpc: "2.0", id, result };
}

function jsonRpcError(id: unknown, code: number, message: string) {
  return { jsonrpc: "2.0", id, error: { code, message } };
}

async function handleJsonRpc(req: JsonRpcRequest) {
  const { id, method, params } = req;

  switch (method) {
    case "initialize":
      return jsonRpcResponse(id, {
        protocolVersion: "2024-11-05",
        capabilities: { tools: {} },
        serverInfo: { name: "cq-mcp", version: "1.0.0" },
      });

    case "notifications/initialized":
      return null; // notification, no response

    case "tools/list":
      return jsonRpcResponse(id, { tools: TOOLS });

    case "tools/call": {
      const toolName = (params as Record<string, unknown>)?.name as string;
      const toolArgs = ((params as Record<string, unknown>)?.arguments ?? {}) as Record<string, unknown>;

      let result: string;
      switch (toolName) {
        case "cq_knowledge_record":
          result = await handleKnowledgeRecord(toolArgs);
          break;
        case "cq_knowledge_search":
          result = await handleRecall(toolArgs);
          break;
        case "cq_status":
          result = await handleStatus();
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

// --- HTTP handler ---

const RESOURCE_URL = "https://mcp.pilab.kr/functions/v1/mcp-server";
const AUTH_SERVER = "https://fhuomvsswxiwbfqjsgit.supabase.co/auth/v1";

Deno.serve(async (req: Request) => {
  // CORS
  if (req.method === "OPTIONS") {
    return new Response(null, {
      headers: {
        "Access-Control-Allow-Origin": "*",
        "Access-Control-Allow-Methods": "GET, POST, OPTIONS",
        "Access-Control-Allow-Headers": "Content-Type, Authorization, X-API-Key",
      },
    });
  }

  // OAuth 2.0 Protected Resource Metadata (RFC 9728)
  const url = new URL(req.url);
  if (
    url.pathname.endsWith("/.well-known/oauth-protected-resource") ||
    url.searchParams.get("path") === ".well-known/oauth-protected-resource"
  ) {
    return new Response(
      JSON.stringify({
        resource: RESOURCE_URL,
        authorization_servers: [AUTH_SERVER],
        bearer_methods_supported: ["header"],
        scopes_supported: ["openid"],
      }),
      { headers: { "Content-Type": "application/json" } },
    );
  }

  // Proxy Authorization Server Metadata — ChatGPT looks for this on the MCP domain
  if (
    url.pathname.includes("/.well-known/oauth-authorization-server") ||
    url.searchParams.get("path") === ".well-known/oauth-authorization-server"
  ) {
    const asRes = await fetch(
      `${AUTH_SERVER}/../.well-known/oauth-authorization-server/auth/v1`,
    );
    const asBody = await asRes.text();
    return new Response(asBody, {
      status: asRes.status,
      headers: { "Content-Type": "application/json" },
    });
  }

  // Proxy OpenID Configuration — ChatGPT may also check this
  if (
    url.pathname.includes("/.well-known/openid-configuration") ||
    url.searchParams.get("path") === ".well-known/openid-configuration"
  ) {
    const oidcRes = await fetch(`${AUTH_SERVER}/.well-known/openid-configuration`);
    const oidcBody = await oidcRes.text();
    return new Response(oidcBody, {
      status: oidcRes.status,
      headers: { "Content-Type": "application/json" },
    });
  }

  const supabase = getSupabase();

  // Auth: URL token, API key header, or JWT
  const urlToken = url.searchParams.get("token");
  const apiKey = req.headers.get("X-API-Key");
  const authHeader = req.headers.get("Authorization");
  const bearerToken = authHeader?.startsWith("Bearer ") ? authHeader.slice(7) : undefined;

  let authorized = false;

  // 1. URL token (for ChatGPT which can't send custom headers)
  if (MCP_API_KEY && urlToken === MCP_API_KEY) {
    authorized = true;
  }

  // 2. API key header (for Claude Code .mcp.json)
  if (!authorized && MCP_API_KEY && (apiKey === MCP_API_KEY || bearerToken === MCP_API_KEY)) {
    authorized = true;
  }

  // 3. JWT (for OAuth flow, future use)
  if (!authorized && bearerToken) {
    const { error } = await supabase.auth.getUser(bearerToken);
    if (!error) {
      authorized = true;
    }
  }

  // Allow discovery methods without auth (needed for connector setup)
  if (!authorized && req.method === "POST") {
    try {
      const bodyText = await req.text();
      const bodyJson = JSON.parse(bodyText);
      const method = bodyJson.method;
      if (method === "initialize" || method === "tools/list" || method === "notifications/initialized") {
        authorized = true;
      }
      (req as any)._bodyText = bodyText;
    } catch { /* not JSON */ }
  }

  // GET (SSE keepalive, metadata) allowed without auth
  if (!authorized && req.method === "GET") {
    authorized = true;
  }

  if (!authorized) {
    return new Response(JSON.stringify({ error: "unauthorized" }), {
      status: 401,
      headers: {
        "Content-Type": "application/json",
        "WWW-Authenticate": `Bearer resource_metadata="${RESOURCE_URL}/.well-known/oauth-protected-resource"`,
      },
    });
  }

  // GET: SSE keepalive (Streamable HTTP spec)
  if (req.method === "GET") {
    const body = new ReadableStream({
      start(controller) {
        const interval = setInterval(() => {
          controller.enqueue(new TextEncoder().encode(": keepalive\n\n"));
        }, 15000);
        req.signal.addEventListener("abort", () => {
          clearInterval(interval);
          controller.close();
        });
      },
    });
    return new Response(body, {
      headers: {
        "Content-Type": "text/event-stream",
        "Cache-Control": "no-cache",
        "Connection": "keep-alive",
      },
    });
  }

  // POST: JSON-RPC
  if (req.method === "POST") {
    let rpcReq: JsonRpcRequest;
    try {
      // Use cached body if already consumed by auth check, otherwise read fresh
      const bodyText = (req as any)._bodyText ?? await req.text();
      rpcReq = JSON.parse(bodyText);
    } catch {
      return new Response(
        JSON.stringify(jsonRpcError(null, -32700, "Parse error")),
        { headers: { "Content-Type": "application/json" } },
      );
    }

    // Notification (no id)
    if (rpcReq.id === undefined || rpcReq.id === null) {
      await handleJsonRpc(rpcReq); // fire and forget
      return new Response(null, { status: 202 });
    }

    const result = await handleJsonRpc(rpcReq);
    if (result === null) {
      return new Response(null, { status: 202 });
    }
    return new Response(JSON.stringify(result), {
      headers: { "Content-Type": "application/json" },
    });
  }

  return new Response("Method not allowed", { status: 405 });
});
