// llm-proxy: Supabase Edge Function
// Proxies Anthropic API requests on behalf of authenticated users.
// JWT auth required. Only claude-haiku-4-5 is allowed (cost control).
// Freemium: 100 calls/month per user. Exceeds → 429.

import { createClient } from "https://esm.sh/@supabase/supabase-js@2";

const SUPABASE_URL = Deno.env.get("SUPABASE_URL") ?? "";
const SUPABASE_ANON_KEY = Deno.env.get("SUPABASE_ANON_KEY") ?? "";
const SUPABASE_SERVICE_ROLE_KEY = Deno.env.get("SUPABASE_SERVICE_ROLE_KEY") ?? "";
const ANTHROPIC_API_KEY = Deno.env.get("ANTHROPIC_API_KEY") ?? "";

const ALLOWED_MODELS = new Set([
  "claude-haiku-4-5-20251001",
  "claude-haiku-4-5",
]);
const ANTHROPIC_API_URL = "https://api.anthropic.com/v1/messages";
const FREE_TIER_LIMIT = 100;

function currentMonth(): string {
  const d = new Date();
  return `${d.getUTCFullYear()}-${String(d.getUTCMonth() + 1).padStart(2, "0")}`;
}

Deno.serve(async (req: Request) => {
  if (req.method !== "POST") {
    return new Response(JSON.stringify({ error: "Method not allowed" }), {
      status: 405,
      headers: { "Content-Type": "application/json" },
    });
  }

  // Extract JWT
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

  // Rate limit check (service_role client for llm_usage table)
  const admin = createClient(SUPABASE_URL, SUPABASE_SERVICE_ROLE_KEY);
  const month = currentMonth();
  const { data: usage } = await admin
    .from("llm_usage")
    .select("count")
    .eq("user_id", user.id)
    .eq("month", month)
    .single();

  const currentCount = usage?.count ?? 0;
  const remaining = Math.max(0, FREE_TIER_LIMIT - currentCount);

  if (currentCount >= FREE_TIER_LIMIT) {
    return new Response(
      JSON.stringify({ error: "Free tier limit reached. 100 calls/month. Upgrade to Pro for unlimited." }),
      {
        status: 429,
        headers: {
          "Content-Type": "application/json",
          "X-RateLimit-Limit": String(FREE_TIER_LIMIT),
          "X-RateLimit-Remaining": "0",
        },
      },
    );
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

  // Increment usage counter on successful Anthropic response
  if (anthropicResp.ok) {
    await admin.rpc("increment_llm_usage", {
      p_user_id: user.id,
      p_month: month,
    }).catch(() => {
      // Non-fatal: if counter fails, still return the response
    });
  }

  const respBody = await anthropicResp.text();
  return new Response(respBody, {
    status: anthropicResp.status,
    headers: {
      "Content-Type": "application/json",
      "X-RateLimit-Limit": String(FREE_TIER_LIMIT),
      "X-RateLimit-Remaining": String(remaining - (anthropicResp.ok ? 1 : 0)),
    },
  });
});
