// telegram-notify: Supabase Edge Function
// Triggered by DB webhook when hub_jobs.status changes to 'COMPLETE' (or any terminal state).
// Sends a Telegram message via Bot API.

import { serve } from "https://deno.land/std@0.168.0/http/server.ts";

const BOT_TOKEN = Deno.env.get("TELEGRAM_BOT_TOKEN") ?? "";
const CHAT_ID = Deno.env.get("TELEGRAM_CHAT_ID") ?? "";

interface JobRecord {
  id: string;
  name: string;
  status: string;
  worker_id: string;
  exit_code: number | null;
  result: string;
  started_at: string | null;
  finished_at: string | null;
}

interface WebhookPayload {
  type: "INSERT" | "UPDATE" | "DELETE";
  table: string;
  record: JobRecord;
  old_record: JobRecord | null;
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

  const { type, record, old_record } = payload;

  // Only act on UPDATE where status changed to a terminal state
  if (type !== "UPDATE" || !record) {
    return new Response("OK", { status: 200 });
  }

  const terminal = ["COMPLETE", "FAILED", "CANCELLED"];
  const isTerminal = terminal.includes(record.status);
  const wasAlreadyTerminal = old_record ? terminal.includes(old_record.status) : false;

  if (!isTerminal || wasAlreadyTerminal) {
    return new Response("OK", { status: 200 });
  }

  const statusEmoji: Record<string, string> = {
    COMPLETE: "✅",
    FAILED: "❌",
    CANCELLED: "⚠️",
  };

  const emoji = statusEmoji[record.status] ?? "ℹ️";
  const duration = formatDuration(record.started_at, record.finished_at);
  const exitCode = record.exit_code !== null ? ` (exit ${record.exit_code})` : "";

  const text =
    `${emoji} <b>Job ${record.status}${exitCode}</b>\n` +
    `<b>Name:</b> ${record.name}\n` +
    `<b>ID:</b> <code>${record.id}</code>\n` +
    `<b>Worker:</b> ${record.worker_id || "—"}\n` +
    `<b>Duration:</b> ${duration}`;

  await sendTelegramMessage(text);

  return new Response("OK", { status: 200 });
});
