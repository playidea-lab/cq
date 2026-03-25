// mcp-server: Supabase Edge Function
// Streamable HTTP MCP server for ChatGPT/Claude remote connections.
// Exposes CQ knowledge tools (snapshot, recall, status) via JSON-RPC 2.0.

import { createClient } from "https://esm.sh/@supabase/supabase-js@2";

const SUPABASE_URL = Deno.env.get("SUPABASE_URL") ?? "";
const SUPABASE_SERVICE_ROLE_KEY = Deno.env.get("SUPABASE_SERVICE_ROLE_KEY") ?? "";
const MCP_API_KEY = Deno.env.get("MCP_API_KEY") ?? "";

function getSupabase() {
  return createClient(SUPABASE_URL, SUPABASE_SERVICE_ROLE_KEY);
}

// --- Tool definitions ---

const TOOLS = [
  {
    name: "cq_snapshot",
    description: "Persist a structured snapshot of the current conversation to the CQ knowledge base — an external memory shared across all LLMs. Call this when the user explicitly asks to save, remember, bookmark, or preserve something, OR when a conversation reaches a meaningful conclusion (architecture decision, bug root-cause found, plan finalized) and the user wants it captured. The snapshot becomes retrievable by any LLM via cq_recall, enabling cross-session and cross-LLM continuity. Focus on capturing the WHY and WHAT-WAS-DECIDED, not a transcript.",
    inputSchema: {
      type: "object",
      required: ["summary"],
      properties: {
        summary: {
          type: "string",
          description: "A 1-3 sentence distillation of the conversation's outcome. Lead with the conclusion or decision, not the process. Write as if a different LLM will read this cold in a future session. Bad: 'We talked about auth.' Good: 'Decided to replace session-cookie auth with short-lived JWTs (15 min) + refresh tokens stored in HttpOnly cookies, driven by compliance requirement from legal.'",
        },
        decisions: {
          type: "array",
          items: { type: "string" },
          description: "Concrete decisions or conclusions reached. Each entry should be self-contained and actionable. Include the rationale when non-obvious. Example: 'Use Postgres advisory locks instead of Redis for distributed locking — fewer moving parts, acceptable at current scale.'",
        },
        open_questions: {
          type: "array",
          items: { type: "string" },
          description: "Unresolved questions, blocked items, or explicitly deferred decisions. These tell the next session where to pick up. Example: 'Need to benchmark advisory lock contention above 50 concurrent workers before committing.'",
        },
        source_llm: {
          type: "string",
          description: "Identifier of the LLM producing this snapshot. Use your model name (e.g., 'claude-sonnet-4-20250514', 'gpt-4o'). This enables provenance tracking across LLMs.",
        },
        tags: {
          type: "array",
          items: { type: "string" },
          description: "Short lowercase labels for retrieval. Include: the domain area (e.g., 'auth', 'database', 'ci'), the type of knowledge (e.g., 'decision', 'debug', 'design'), and any project-specific identifiers. 2-5 tags is ideal.",
        },
      },
    },
  },
  {
    name: "cq_recall",
    description: "Search the CQ knowledge base for previously saved snapshots and context from past conversations — including those from other LLMs. Call this when the user asks to recall, continue, resume, or reference prior work ('what did we decide about X', 'pick up where we left off', 'what's the context on Y'). Also call proactively when the user starts a task that likely has prior context (e.g., 'let's finish the auth migration') — checking for existing knowledge prevents redundant work and contradictory decisions.",
    inputSchema: {
      type: "object",
      required: ["query"],
      properties: {
        query: {
          type: "string",
          description: "A natural-language search query describing the knowledge you need. Be specific about the domain and intent. Bad: 'auth'. Good: 'authentication migration decision — JWT vs session cookies'. Include synonyms or related terms if the exact phrasing is uncertain.",
        },
        limit: {
          type: "number",
          description: "Maximum number of snapshots to return. Use 1-3 for targeted lookups ('what did we decide about X'), 5-10 for broader context gathering ('everything related to the payment system'). Default: 5.",
        },
      },
    },
  },
  {
    name: "cq_status",
    description: "Get a real-time overview of the CQ project: task counts by state, active workers, and overall progress. Call this when the user asks about project status, progress, what's in flight, or what's left to do. Also useful at the start of a session to orient yourself before taking on new work.",
    inputSchema: {
      type: "object",
      properties: {},
    },
  },
];

// --- Tool implementations ---

async function handleSnapshot(args: Record<string, unknown>): Promise<string> {
  const supabase = getSupabase();
  const summary = args.summary as string;
  const decisions = (args.decisions as string[]) || [];
  const openQuestions = (args.open_questions as string[]) || [];
  const sourceLlm = (args.source_llm as string) || "unknown";
  const tags = (args.tags as string[]) || [];

  const content = [
    `## Snapshot: ${summary}`,
    "",
    `**Source**: ${sourceLlm}`,
    `**Date**: ${new Date().toISOString()}`,
    "",
    decisions.length > 0 ? `### Decisions\n${decisions.map(d => `- ${d}`).join("\n")}` : "",
    openQuestions.length > 0 ? `### Open Questions\n${openQuestions.map(q => `- ${q}`).join("\n")}` : "",
    tags.length > 0 ? `\n**Tags**: ${tags.join(", ")}` : "",
  ].filter(Boolean).join("\n");

  const docId = `snap-${crypto.randomUUID().slice(0, 8)}`;
  const { data, error } = await supabase
    .from("c4_documents")
    .insert({
      doc_id: docId,
      project_id: Deno.env.get("C4_PROJECT_ID") ?? "00000000-0000-0000-0000-000000000000",
      title: `Snapshot: ${summary.slice(0, 80)}`,
      body: content,
      doc_type: "insight",
      domain: "snapshot",
      tags: JSON.stringify(["snapshot", sourceLlm, ...tags]),
      created_by: sourceLlm,
    })
    .select("doc_id")
    .single();

  if (error) return JSON.stringify({ error: error.message });
  return JSON.stringify({ success: true, id: data.doc_id, message: `Snapshot saved: ${summary.slice(0, 50)}` });
}

async function handleRecall(args: Record<string, unknown>): Promise<string> {
  const supabase = getSupabase();
  const query = args.query as string;
  const limit = (args.limit as number) || 5;

  const projectId = Deno.env.get("C4_PROJECT_ID") ?? "00000000-0000-0000-0000-000000000000";

  // Full-text search on documents, filtered to snapshots
  const { data, error } = await supabase
    .from("c4_documents")
    .select("doc_id, title, body, tags, created_at")
    .eq("project_id", projectId)
    .eq("domain", "snapshot")
    .textSearch("tsv", query.split(" ").join(" & "))
    .order("created_at", { ascending: false })
    .limit(limit);

  if (error) {
    // Fallback: simple ilike search
    const { data: fallback, error: fbErr } = await supabase
      .from("c4_documents")
      .select("doc_id, title, body, tags, created_at")
      .eq("project_id", projectId)
      .eq("domain", "snapshot")
      .ilike("body", `%${query}%`)
      .order("created_at", { ascending: false })
      .limit(limit);

    if (fbErr) return JSON.stringify({ error: fbErr.message });
    return JSON.stringify({ results: fallback || [], count: fallback?.length || 0 });
  }

  return JSON.stringify({ results: data || [], count: data?.length || 0 });
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
        case "cq_snapshot":
          result = await handleSnapshot(toolArgs);
          break;
        case "cq_recall":
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

  const supabase = getSupabase();

  // Auth check: skip for discovery methods (initialize, tools/list), require for tool calls
  const apiKey = req.headers.get("X-API-Key");
  const authHeader = req.headers.get("Authorization");
  const bearerToken = authHeader?.startsWith("Bearer ") ? authHeader.slice(7) : undefined;

  let authorized = false;

  // Check API key
  if (MCP_API_KEY && (apiKey === MCP_API_KEY || bearerToken === MCP_API_KEY)) {
    authorized = true;
  }

  // Check JWT
  if (!authorized && bearerToken) {
    const { error } = await supabase.auth.getUser(bearerToken);
    if (!error) {
      authorized = true;
    }
  }

  // Allow unauthenticated access to discovery methods (initialize, tools/list, notifications/initialized)
  // These are needed for ChatGPT connector setup before OAuth completes
  if (!authorized && req.method === "POST") {
    try {
      const bodyText = await req.text();
      const bodyJson = JSON.parse(bodyText);
      const method = bodyJson.method;
      if (method === "initialize" || method === "tools/list" || method === "notifications/initialized" || method === "tools/call") {
        authorized = true;
      }
      // Store body for later use since we consumed it
      (req as any)._bodyText = bodyText;
    } catch { /* not JSON, will fail later */ }
  }

  // GET (SSE) is allowed without auth
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
