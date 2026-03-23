// llm-proxy: Supabase Edge Function
// Proxies Anthropic API requests on behalf of authenticated users.
// JWT auth required. Only claude-haiku-4-5 is allowed (cost control).

import { createClient } from "https://esm.sh/@supabase/supabase-js@2";

const SUPABASE_URL = Deno.env.get("SUPABASE_URL") ?? "";
const SUPABASE_ANON_KEY = Deno.env.get("SUPABASE_ANON_KEY") ?? "";
const ANTHROPIC_API_KEY = Deno.env.get("ANTHROPIC_API_KEY") ?? "";

const ALLOWED_MODELS = new Set(["claude-haiku-4-5"]);
const ANTHROPIC_API_URL = "https://api.anthropic.com/v1/messages";

Deno.serve(async (req: Request) => {
  // Only POST is supported
  if (req.method !== "POST") {
    return new Response(JSON.stringify({ error: "Method not allowed" }), {
      status: 405,
      headers: { "Content-Type": "application/json" },
    });
  }

  // Extract JWT from Authorization header
  const authHeader = req.headers.get("Authorization");
  if (!authHeader || !authHeader.startsWith("Bearer ")) {
    return new Response(JSON.stringify({ error: "Missing authorization header" }), {
      status: 401,
      headers: { "Content-Type": "application/json" },
    });
  }
  const jwt = authHeader.slice("Bearer ".length);

  // Verify JWT via Supabase Auth
  const supabase = createClient(SUPABASE_URL, SUPABASE_ANON_KEY, {
    global: { headers: { Authorization: `Bearer ${jwt}` } },
  });
  const { data: { user }, error: authError } = await supabase.auth.getUser();
  if (authError || !user) {
    return new Response(JSON.stringify({ error: "Unauthorized" }), {
      status: 401,
      headers: { "Content-Type": "application/json" },
    });
  }

  // Parse request body
  let body: { model?: string; max_tokens?: number; messages?: unknown; system?: string };
  try {
    body = await req.json();
  } catch {
    return new Response(JSON.stringify({ error: "Invalid JSON body" }), {
      status: 400,
      headers: { "Content-Type": "application/json" },
    });
  }

  // Enforce model whitelist
  const { model, max_tokens, messages, system } = body;
  if (!model || !ALLOWED_MODELS.has(model)) {
    return new Response(
      JSON.stringify({ error: `Model not allowed. Use: ${[...ALLOWED_MODELS].join(", ")}` }),
      { status: 400, headers: { "Content-Type": "application/json" } },
    );
  }

  if (!messages || !max_tokens) {
    return new Response(JSON.stringify({ error: "Missing required fields: messages, max_tokens" }), {
      status: 400,
      headers: { "Content-Type": "application/json" },
    });
  }

  // Proxy to Anthropic API
  const anthropicPayload: Record<string, unknown> = { model, max_tokens, messages };
  if (system) anthropicPayload.system = system;

  const anthropicResp = await fetch(ANTHROPIC_API_URL, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      "x-api-key": ANTHROPIC_API_KEY,
      "anthropic-version": "2023-06-01",
    },
    body: JSON.stringify(anthropicPayload),
  });

  const respBody = await anthropicResp.text();
  return new Response(respBody, {
    status: anthropicResp.status,
    headers: { "Content-Type": "application/json" },
  });
});
