// telegram-notify: Supabase Edge Function
// Triggered by DB webhooks on c4_tasks and hub_jobs status changes.
// Sends Telegram messages via Bot API.

import { serve } from "https://deno.land/std@0.168.0/http/server.ts";

const BOT_TOKEN = Deno.env.get("TELEGRAM_BOT_TOKEN") ?? "";
const CHAT_ID = Deno.env.get("TELEGRAM_CHAT_ID") ?? "";

interface WebhookPayload {
  type: "INSERT" | "UPDATE" | "DELETE";
  table: string;
  schema: string;
  record: Record<string, unknown>;
  old_record: Record<string, unknown> | null;
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

function handleTaskUpdate(record: Record<string, unknown>, oldRecord: Record<string, unknown> | null): string | null {
  const status = record.status as string;
  const oldStatus = oldRecord?.status as string | undefined;

  // Only notify on terminal transitions
  if (oldStatus === status) return null;

  const taskId = record.task_id as string;
  const title = record.title as string;

  switch (status) {
    case "done":
      return `✅ <b>Task done</b>\n<b>${taskId}</b>: ${title}`;
    case "blocked": {
      const reason = (record.failure_signature as string) || (record.last_error as string) || "unknown";
      return `🚫 <b>Task blocked</b>\n<b>${taskId}</b>: ${title}\n<i>${reason}</i>`;
    }
    case "in_progress":
      // Silent — no notification for in_progress
      return null;
    default:
      return null;
  }
}

function handleJobUpdate(record: Record<string, unknown>, oldRecord: Record<string, unknown> | null): string | null {
  const status = record.status as string;

  const terminal = ["COMPLETE", "FAILED", "CANCELLED"];
  if (!terminal.includes(status)) return null;

  const oldStatus = oldRecord?.status as string | undefined;
  if (oldStatus && terminal.includes(oldStatus)) return null; // already terminal

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

  return (
    `${emoji} <b>Job ${status}${exitCode}</b>\n` +
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
      message = handleTaskUpdate(record, old_record);
      break;
    case "hub_jobs":
      message = handleJobUpdate(record, old_record);
      break;
    default:
      console.log(`Unknown table: ${table}`);
  }

  if (message) {
    await sendTelegramMessage(message);
  }

  return new Response("OK", { status: 200 });
});
