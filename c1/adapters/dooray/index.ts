/**
 * c1/adapters/dooray/index.ts
 * Dooray Messenger adapter implementing PlatformAdapter.
 *
 * Inbound:  HTTP POST localhost:{port} — Dooray outgoing webhook payload
 * Outbound: Dooray REST API POST /messenger/v1/channels/direct-send
 *
 * Env:
 *   DOORAY_BOT_TOKEN   — Bearer token for Dooray REST API
 *   DOORAY_TENANT_ID   — Dooray tenant identifier
 *   DOORAY_API_URL     — Base URL (default: https://api.dooray.com)
 *   DOORAY_LISTEN_PORT — Local webhook listener port (default: 9981)
 */

import { type PlatformAdapter, type AdapterConfig, type InboundMessage } from "../../core/adapter.js";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

/** Shape of an inbound Dooray outgoing webhook POST body */
export interface DoorayWebhookBody {
  tenantId?: string;
  channelId?: string;
  senderId?: string;
  text?: string;
  [key: string]: unknown;
}

/** Options for DoorayAdapter constructor */
export interface DoorayAdapterOptions {
  /** Dooray Bearer token — falls back to DOORAY_BOT_TOKEN env var */
  botToken?: string;
  /** Dooray tenant ID — falls back to DOORAY_TENANT_ID env var */
  tenantId?: string;
  /** Dooray API base URL — falls back to DOORAY_API_URL env var */
  apiUrl?: string;
  /** Local listener port — falls back to DOORAY_LISTEN_PORT env var (default 9981) */
  listenPort?: number;
}

// ---------------------------------------------------------------------------
// DoorayAdapter
// ---------------------------------------------------------------------------

export class DoorayAdapter implements PlatformAdapter {
  readonly config: AdapterConfig;

  private readonly botToken: string;
  private readonly tenantId: string;
  private readonly apiUrl: string;
  private readonly listenPort: number;

  private handlers: Array<(msg: InboundMessage) => void> = [];
  private server: ReturnType<typeof Bun.serve> | null = null;

  constructor(options: DoorayAdapterOptions = {}) {
    this.botToken =
      options.botToken ?? process.env.DOORAY_BOT_TOKEN ?? "";
    this.tenantId =
      options.tenantId ?? process.env.DOORAY_TENANT_ID ?? "";
    this.apiUrl =
      options.apiUrl ?? process.env.DOORAY_API_URL ?? "https://api.dooray.com";
    this.listenPort =
      options.listenPort ?? Number(process.env.DOORAY_LISTEN_PORT ?? "9981");

    this.config = {
      id: "dooray",
      name: "Dooray Messenger",
      options: {
        tenantId: this.tenantId,
        apiUrl: this.apiUrl,
        listenPort: this.listenPort,
      },
    };
  }

  // ---------------------------------------------------------------------------
  // PlatformAdapter — lifecycle
  // ---------------------------------------------------------------------------

  async initialize(): Promise<void> {
    if (this.server) return; // idempotent

    this.server = Bun.serve({
      port: this.listenPort,
      fetch: async (req) => {
        if (req.method !== "POST") {
          return new Response("method not allowed", { status: 405 });
        }

        let body: DoorayWebhookBody;
        try {
          body = (await req.json()) as DoorayWebhookBody;
        } catch {
          return new Response("bad request", { status: 400 });
        }

        const inbound: InboundMessage = {
          type: "dooray.message",
          source: "dooray",
          timestamp: Date.now(),
          payload: {
            chat_id: body.channelId ?? "",
            sender: body.senderId ?? "",
            text: body.text ?? "",
            raw: body,
          },
        };

        for (const h of this.handlers) {
          h(inbound);
        }

        return new Response("ok", { status: 200 });
      },
    });
  }

  async destroy(): Promise<void> {
    if (!this.server) return; // idempotent
    this.server.stop();
    this.server = null;
    this.handlers = [];
  }

  // ---------------------------------------------------------------------------
  // PlatformAdapter — send
  // ---------------------------------------------------------------------------

  /**
   * Send a reply to a Dooray channel.
   * `message` must contain `chat_id` (channelId) and `text`.
   */
  async send(message: unknown): Promise<boolean> {
    const { chat_id, text } = message as { chat_id: string; text: string };

    const url = `${this.apiUrl}/messenger/v1/channels/${encodeURIComponent(chat_id)}/direct-send`;

    try {
      const resp = await fetch(url, {
        method: "POST",
        headers: {
          "Authorization": `dooray-api ${this.botToken}`,
          "Content-Type": "application/json",
        },
        body: JSON.stringify({
          tenantId: this.tenantId,
          channelId: chat_id,
          text,
        }),
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
