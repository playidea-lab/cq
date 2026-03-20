/**
 * c1/adapters/dooray/index.ts
 * Dooray Messenger adapter implementing PlatformAdapter.
 *
 * Inbound:  Polls Hub API GET /v1/dooray/pending (3s interval)
 * Outbound: Hub API POST /v1/dooray/reply → response_url proxy
 *
 * Env:
 *   C1_HUB_URL         — Hub base URL (default: http://localhost:4142)
 *   DOORAY_BOT_TOKEN   — (legacy, unused by polling mode)
 *   DOORAY_TENANT_ID   — (legacy, unused by polling mode)
 */

import { type PlatformAdapter, type AdapterConfig, type InboundMessage } from "../../core/adapter.js";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

/** Shape of a message from Hub /v1/dooray/pending */
export interface DoorayPendingMessage {
  channelId?: string;
  senderId?: string;
  text?: string;
  response_url?: string;
  [key: string]: unknown;
}

/** Options for DoorayAdapter constructor */
export interface DoorayAdapterOptions {
  /** Hub API base URL — falls back to C1_HUB_URL env var */
  hubUrl?: string;
  /** Polling interval in ms (default: 3000) */
  pollIntervalMs?: number;
}

// ---------------------------------------------------------------------------
// DoorayAdapter
// ---------------------------------------------------------------------------

export class DoorayAdapter implements PlatformAdapter {
  readonly config: AdapterConfig;

  private readonly hubUrl: string;
  private readonly pollIntervalMs: number;

  private handlers: Array<(msg: InboundMessage) => void> = [];
  private pollTimer: ReturnType<typeof setInterval> | null = null;
  private polling = false;

  constructor(options: DoorayAdapterOptions = {}) {
    this.hubUrl =
      options.hubUrl ?? process.env.C1_HUB_URL ?? "http://localhost:4142";
    this.pollIntervalMs = options.pollIntervalMs ?? 3000;

    this.config = {
      id: "dooray",
      name: "Dooray Messenger (Hub polling)",
      options: {
        hubUrl: this.hubUrl,
        pollIntervalMs: this.pollIntervalMs,
      },
    };
  }

  // ---------------------------------------------------------------------------
  // PlatformAdapter — lifecycle
  // ---------------------------------------------------------------------------

  async initialize(): Promise<void> {
    if (this.pollTimer) return; // idempotent

    this.pollTimer = setInterval(() => this.poll(), this.pollIntervalMs);
    console.error(`Dooray adapter polling ${this.hubUrl}/v1/dooray/pending every ${this.pollIntervalMs}ms`);
  }

  async destroy(): Promise<void> {
    if (this.pollTimer) {
      clearInterval(this.pollTimer);
      this.pollTimer = null;
    }
    this.handlers = [];
  }

  // ---------------------------------------------------------------------------
  // Polling loop
  // ---------------------------------------------------------------------------

  private async poll(): Promise<void> {
    if (this.polling) return; // skip if previous poll still running
    this.polling = true;
    try {
      const resp = await fetch(`${this.hubUrl}/v1/dooray/pending`, {
        signal: AbortSignal.timeout(5000),
      });
      if (!resp.ok) return;

      const messages: DoorayPendingMessage[] = await resp.json() as DoorayPendingMessage[];
      if (!Array.isArray(messages) || messages.length === 0) return;

      for (const msg of messages) {
        const inbound: InboundMessage = {
          type: "dooray.message",
          source: "dooray",
          timestamp: Date.now(),
          payload: {
            chat_id: msg.channelId ?? "",
            sender: msg.senderId ?? "",
            text: msg.text ?? "",
            response_url: msg.response_url ?? "",
            raw: msg,
          },
        };
        for (const h of this.handlers) {
          h(inbound);
        }
      }
    } catch {
      // Network error or timeout — silently skip, retry next interval
    } finally {
      this.polling = false;
    }
  }

  // ---------------------------------------------------------------------------
  // PlatformAdapter — send (via Hub reply proxy)
  // ---------------------------------------------------------------------------

  async send(message: unknown): Promise<boolean> {
    const { text } = message as { chat_id?: string; text: string };

    try {
      const resp = await fetch(`${this.hubUrl}/v1/dooray/reply`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ text }),
        signal: AbortSignal.timeout(10000),
      });
      return resp.ok;
    } catch {
      return false;
    }
  }

  // ---------------------------------------------------------------------------
  // PlatformAdapter — onMessage
  // ---------------------------------------------------------------------------

  onMessage(handler: (msg: InboundMessage) => void): () => void {
    this.handlers.push(handler);
    return () => {
      this.handlers = this.handlers.filter((h) => h !== handler);
    };
  }
}
