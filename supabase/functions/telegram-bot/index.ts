// telegram-bot: Supabase Edge Function
// Telegram webhook endpoint — handles slash commands from the bot.
// Commands: /status, /cancel <id>, /workers, /jobs
// Uses grammY framework + Supabase client for DB access.

import { Bot, webhookCallback } from "https://deno.land/x/grammy@v1.21.1/mod.ts";
import { createClient } from "https://esm.sh/@supabase/supabase-js@2";

const BOT_TOKEN = Deno.env.get("TELEGRAM_BOT_TOKEN") ?? "";
const SUPABASE_URL = Deno.env.get("SUPABASE_URL") ?? "";
const SUPABASE_SERVICE_ROLE_KEY = Deno.env.get("SUPABASE_SERVICE_ROLE_KEY") ?? "";

if (!BOT_TOKEN) throw new Error("TELEGRAM_BOT_TOKEN is required");

const bot = new Bot(BOT_TOKEN);

function getSupabase() {
  return createClient(SUPABASE_URL, SUPABASE_SERVICE_ROLE_KEY);
}

// /status — active jobs (QUEUED or RUNNING)
bot.command("status", async (ctx) => {
  const supabase = getSupabase();
  const { data: jobs, error } = await supabase
    .from("hub_jobs")
    .select("id, name, status, worker_id, created_at")
    .in("status", ["QUEUED", "RUNNING"])
    .order("created_at", { ascending: true });

  if (error) {
    await ctx.reply(`Error: ${error.message}`);
    return;
  }

  if (!jobs || jobs.length === 0) {
    await ctx.reply("No active jobs.");
    return;
  }

  const lines = jobs.map(
    (j: { status: string; name: string; id: string; worker_id: string }) =>
      `• [${j.status}] ${j.name} — <code>${j.id}</code>` +
      (j.worker_id ? ` (${j.worker_id})` : ""),
  );
  await ctx.reply(`<b>Active Jobs (${jobs.length})</b>\n${lines.join("\n")}`, {
    parse_mode: "HTML",
  });
});

// /cancel <id> — cancel a job
bot.command("cancel", async (ctx) => {
  const jobId = ctx.match?.trim();
  if (!jobId) {
    await ctx.reply("Usage: /cancel <job_id>");
    return;
  }

  const supabase = getSupabase();
  const { data, error } = await supabase
    .from("hub_jobs")
    .update({ status: "CANCELLED" })
    .eq("id", jobId)
    .in("status", ["QUEUED", "RUNNING"])
    .select("id, name")
    .single();

  if (error || !data) {
    await ctx.reply(`Could not cancel job <code>${jobId}</code>. Check ID or job may already be terminal.`, {
      parse_mode: "HTML",
    });
    return;
  }

  await ctx.reply(`⚠️ Cancelled job <b>${data.name}</b> (<code>${data.id}</code>)`, {
    parse_mode: "HTML",
  });
});

// /workers — list online workers
bot.command("workers", async (ctx) => {
  const supabase = getSupabase();
  const { data: workers, error } = await supabase
    .from("hub_workers")
    .select("id, hostname, name, status, gpu_model, gpu_count, last_heartbeat")
    .eq("status", "online")
    .order("last_heartbeat", { ascending: false });

  if (error) {
    await ctx.reply(`Error: ${error.message}`);
    return;
  }

  if (!workers || workers.length === 0) {
    await ctx.reply("No online workers.");
    return;
  }

  const lines = workers.map(
    (w: { name: string; hostname: string; id: string; gpu_model: string; gpu_count: number }) => {
      const gpu = w.gpu_count > 0 ? ` 🖥️ ${w.gpu_model} x${w.gpu_count}` : "";
      return `• <b>${w.name}</b> (${w.hostname}) — <code>${w.id}</code>${gpu}`;
    },
  );
  await ctx.reply(`<b>Online Workers (${workers.length})</b>\n${lines.join("\n")}`, {
    parse_mode: "HTML",
  });
});

// /jobs — last 10 jobs
bot.command("jobs", async (ctx) => {
  const supabase = getSupabase();
  const { data: jobs, error } = await supabase
    .from("hub_jobs")
    .select("id, name, status, created_at, finished_at")
    .order("created_at", { ascending: false })
    .limit(10);

  if (error) {
    await ctx.reply(`Error: ${error.message}`);
    return;
  }

  if (!jobs || jobs.length === 0) {
    await ctx.reply("No jobs found.");
    return;
  }

  const statusEmoji: Record<string, string> = {
    COMPLETE: "✅",
    FAILED: "❌",
    CANCELLED: "⚠️",
    RUNNING: "🔄",
    QUEUED: "⏳",
  };

  const lines = jobs.map(
    (j: { status: string; name: string; id: string }) => {
      const emoji = statusEmoji[j.status] ?? "•";
      return `${emoji} ${j.name} — <code>${j.id}</code>`;
    },
  );
  await ctx.reply(`<b>Recent Jobs</b>\n${lines.join("\n")}`, {
    parse_mode: "HTML",
  });
});

const handleUpdate = webhookCallback(bot, "std/http");

Deno.serve(handleUpdate);
