// oauth-token-proxy: Supabase Edge Function
// Proxies OAuth token requests to Supabase Auth, logging request/response for debugging.
// Used to diagnose ChatGPT OAuth token exchange failures.

import { createClient } from "https://esm.sh/@supabase/supabase-js@2";

const SUPABASE_URL = Deno.env.get("SUPABASE_URL") ?? "";
const SUPABASE_SERVICE_ROLE_KEY = Deno.env.get("SUPABASE_SERVICE_ROLE_KEY") ?? "";

function getSupabase() {
  return createClient(SUPABASE_URL, SUPABASE_SERVICE_ROLE_KEY);
}

Deno.serve(async (req: Request) => {
  // CORS
  if (req.method === "OPTIONS") {
    return new Response(null, {
      headers: {
        "Access-Control-Allow-Origin": "*",
        "Access-Control-Allow-Methods": "POST, OPTIONS",
        "Access-Control-Allow-Headers": "Content-Type, Authorization",
      },
    });
  }

  if (req.method !== "POST") {
    return new Response("Method not allowed", { status: 405 });
  }

  // Capture the incoming request
  const contentType = req.headers.get("Content-Type") ?? "";
  const authHeader = req.headers.get("Authorization") ?? "";
  const bodyText = await req.text();

  // Log to c4_documents for persistent debugging
  const supabase = getSupabase();
  const logEntry = {
    timestamp: new Date().toISOString(),
    content_type: contentType,
    auth_header: authHeader ? authHeader.slice(0, 30) + "..." : "none",
    body: bodyText,
    headers: Object.fromEntries(
      [...req.headers.entries()].filter(([k]) => !k.toLowerCase().includes("cookie"))
    ),
  };

  console.log("=== TOKEN PROXY REQUEST ===");
  console.log(JSON.stringify(logEntry, null, 2));

  // Forward to Supabase Auth token endpoint
  const targetUrl = `${SUPABASE_URL}/auth/v1/oauth/token`;
  const forwardHeaders: Record<string, string> = {
    "Content-Type": contentType || "application/x-www-form-urlencoded",
  };
  if (authHeader) {
    forwardHeaders["Authorization"] = authHeader;
  }
  // Supabase needs apikey
  forwardHeaders["apikey"] = Deno.env.get("SUPABASE_ANON_KEY") ?? SUPABASE_SERVICE_ROLE_KEY;

  const targetRes = await fetch(targetUrl, {
    method: "POST",
    headers: forwardHeaders,
    body: bodyText,
  });

  const targetBody = await targetRes.text();
  const targetStatus = targetRes.status;

  console.log("=== TOKEN PROXY RESPONSE ===");
  console.log(`Status: ${targetStatus}`);
  console.log(`Body: ${targetBody.slice(0, 500)}`);

  // Also save to DB for later inspection
  await supabase.from("c4_documents").insert({
    doc_id: `oauth-debug-${crypto.randomUUID().slice(0, 8)}`,
    project_id: Deno.env.get("C4_PROJECT_ID") ?? "00000000-0000-0000-0000-000000000000",
    title: `OAuth token debug ${new Date().toISOString()}`,
    body: JSON.stringify({ request: logEntry, response: { status: targetStatus, body: targetBody } }, null, 2),
    doc_type: "insight",
    domain: "oauth-debug",
    tags: JSON.stringify(["oauth", "debug"]),
    created_by: "token-proxy",
  }).then(() => {}).catch(() => {});

  // Return the response to ChatGPT
  return new Response(targetBody, {
    status: targetStatus,
    headers: {
      "Content-Type": targetRes.headers.get("Content-Type") ?? "application/json",
      "Access-Control-Allow-Origin": "*",
    },
  });
});
