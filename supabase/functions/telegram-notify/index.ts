// telegram-notify: Supabase Edge Function
// Triggered by DB webhooks on c4_tasks and hub_jobs status changes.
// Sends Telegram messages via Bot API with project context.

import { serve } from "https://deno.land/std@0.168.0/http/server.ts";
import { createClient } from "https://esm.sh/@supabase/supabase-js@2";

const BOT_TOKEN = Deno.env.get("TELEGRAM_BOT_TOKEN") ?? "";
const CHAT_ID = Deno.env.get("TELEGRAM_CHAT_ID") ?? "";
const SUPABASE_URL = Deno.env.get("SUPABASE_URL") ?? "";
const SUPABASE_SERVICE_ROLE_KEY = Deno.env.get("SUPABASE_SERVICE_ROLE_KEY") ?? "";

interface WebhookPayload {
  type: "INSERT" | "UPDATE" | "DELETE";
  table: string;
  schema: string;
  record: Record<string, unknown>;
  old_record: Record<string, unknown> | null;
}

// Cache project names to avoid repeated lookups within the same invocation.
const projectNameCache = new Map<string, string>();

async function resolveProjectName(projectId: string): Promise<string> {
  if (!projectId) return "";
  if (projectNameCache.has(projectId)) return projectNameCache.get(projectId)!;

  if (!SUPABASE_URL || !SUPABASE_SERVICE_ROLE_KEY) return projectId.slice(0, 8);

  try {
    const supabase = createClient(SUPABASE_URL, SUPABASE_SERVICE_ROLE_KEY);
    const { data } = await supabase
      .from("c4_projects")
      .select("name")
      .eq("id", projectId)
      .single();
    const name = data?.name ?? projectId.slice(0, 8);
    projectNameCache.set(projectId, name);
    return name;
  } catch {
    return projectId.slice(0, 8);
  }
}

async function sendTelegramMessage(text: string): Promise<void> {
  const url = `https://api.telegram.org/bot${BOT_TOKEN}/sendMessage`;
  const res = await fetch(url, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      chat_id: CHAT_ID,
      text,
      parse_mode: "HTML",
    }),
  });
  if (!res.ok) {
    const body = await res.text();
    console.error("Telegram API error:", res.status, body);
  }
}

function formatDuration(startedAt: string | null, finishedAt: string | null): string {
  if (!startedAt || !finishedAt) return "—";
  const ms = new Date(finishedAt).getTime() - new Date(startedAt).getTime();
  const sec = Math.floor(ms / 1000);
  if (sec < 60) return `${sec}s`;
  const min = Math.floor(sec / 60);
  return `${min}m ${sec % 60}s`;
}

// --- Table-specific handlers ---

async function handleTaskUpdate(
  record: Record<string, unknown>,
  oldRecord: Record<string, unknown> | null,
): Promise<string | null> {
  const status = record.status as string;
  const oldStatus = oldRecord?.status as string | undefined;

  if (oldStatus === status) return null;

  const taskId = record.task_id as string;
  const title = record.title as string;
  const projectId = record.project_id as string;
  const project = await resolveProjectName(projectId);
  const tag = project ? `<b>[${project}]</b> ` : "";

  switch (status) {
    case "done":
      return `${tag}✅ <b>Task done</b>\n<b>${taskId}</b>: ${title}`;
    case "blocked": {
      const reason = (record.failure_signature as string) || (record.last_error as string) || "unknown";
      return `${tag}🚫 <b>Task blocked</b>\n<b>${taskId}</b>: ${title}\n<i>${reason}</i>`;
    }
    default:
      return null;
  }
}

async function handleJobUpdate(
  record: Record<string, unknown>,
  oldRecord: Record<string, unknown> | null,
): Promise<string | null> {
  const status = record.status as string;

  const terminal = ["COMPLETE", "FAILED", "CANCELLED"];
  if (!terminal.includes(status)) return null;

  const oldStatus = oldRecord?.status as string | undefined;
  if (oldStatus && terminal.includes(oldStatus)) return null;

  const statusEmoji: Record<string, string> = {
    COMPLETE: "✅",
    FAILED: "❌",
    CANCELLED: "⚠️",
  };

  const emoji = statusEmoji[status] ?? "ℹ️";
  const name = record.name as string;
  const id = record.id as string;
  const workerId = (record.worker_id as string) || "—";
  const exitCode = record.exit_code !== null ? ` (exit ${record.exit_code})` : "";
  const duration = formatDuration(
    record.started_at as string | null,
    record.finished_at as string | null,
  );

  // hub_jobs may have project context in tags or name
  const projectTag = (record.project_name as string) || "";
  const tag = projectTag ? `<b>[${projectTag}]</b> ` : "";

  return (
    `${tag}${emoji} <b>Job ${status}${exitCode}</b>\n` +
    `<b>Name:</b> ${name}\n` +
    `<b>ID:</b> <code>${id}</code>\n` +
    `<b>Worker:</b> ${workerId}\n` +
    `<b>Duration:</b> ${duration}`
  );
}

serve(async (req: Request) => {
  if (req.method !== "POST") {
    return new Response("Method Not Allowed", { status: 405 });
  }

  if (!BOT_TOKEN || !CHAT_ID) {
    console.error("Missing TELEGRAM_BOT_TOKEN or TELEGRAM_CHAT_ID");
    return new Response("Configuration error", { status: 500 });
  }

  let payload: WebhookPayload;
  try {
    payload = await req.json();
  } catch {
    return new Response("Invalid JSON", { status: 400 });
  }

  const { type, table, record, old_record } = payload;

  if (type !== "UPDATE" || !record) {
    return new Response("OK", { status: 200 });
  }

  let message: string | null = null;

  switch (table) {
    case "c4_tasks":
      message = await handleTaskUpdate(record, old_record);
      break;
    case "hub_jobs":
      message = await handleJobUpdate(record, old_record);
      break;
    default:
      console.log(`Unknown table: ${table}`);
  }

  if (message) {
    await sendTelegramMessage(message);
  }

  return new Response("OK", { status: 200 });
});
