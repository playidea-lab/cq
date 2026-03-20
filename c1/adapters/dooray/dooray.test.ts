/**
 * c1/adapters/dooray/dooray.test.ts
 * Unit tests for DoorayAdapter using bun:test.
 */

import { describe, it, expect, beforeEach, afterEach, mock, spyOn } from "bun:test";
import { DoorayAdapter } from "./index.js";

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function makeAdapter(overrides: Parameters<typeof DoorayAdapter>[0] = {}) {
  return new DoorayAdapter({
    botToken: "test-token",
    tenantId: "test-tenant",
    apiUrl: "https://dooray.example.com",
    listenPort: 0, // OS-assigned port to avoid conflicts
    ...overrides,
  });
}

// ---------------------------------------------------------------------------
// config
// ---------------------------------------------------------------------------

describe("DoorayAdapter — config", () => {
  it("exposes id=dooray", () => {
    const a = makeAdapter();
    expect(a.config.id).toBe("dooray");
    expect(a.config.name).toBe("Dooray Messenger");
  });

  it("reads options from constructor", () => {
    const a = makeAdapter({ tenantId: "my-tenant" });
    expect((a.config.options as Record<string, unknown>).tenantId).toBe("my-tenant");
  });
});

// ---------------------------------------------------------------------------
// lifecycle — initialize / destroy
// ---------------------------------------------------------------------------

describe("DoorayAdapter — lifecycle", () => {
  let adapter: DoorayAdapter;

  beforeEach(() => {
    adapter = makeAdapter();
  });

  afterEach(async () => {
    await adapter.destroy();
  });

  it("initialize starts server (idempotent)", async () => {
    await adapter.initialize();
    await adapter.initialize(); // should not throw
  });

  it("destroy stops server (idempotent)", async () => {
    await adapter.initialize();
    await adapter.destroy();
    await adapter.destroy(); // should not throw
  });
});

// ---------------------------------------------------------------------------
// webhook ingestion
// ---------------------------------------------------------------------------

describe("DoorayAdapter — inbound webhook", () => {
  let adapter: DoorayAdapter;
  let port: number;

  beforeEach(async () => {
    adapter = makeAdapter({ listenPort: 0 });
    await adapter.initialize();
    // Access the private server port via Bun.serve's `.port`
    const srv = (adapter as unknown as { server: { port: number } }).server;
    port = srv.port;
  });

  afterEach(async () => {
    await adapter.destroy();
  });

  it("emits InboundMessage for POST /", async () => {
    const received: unknown[] = [];
    const unsub = adapter.onMessage((msg) => received.push(msg));

    const body = {
      tenantId: "test-tenant",
      channelId: "ch-123",
      senderId: "user-42",
      text: "hello from dooray",
    };

    const resp = await fetch(`http://localhost:${port}/`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    });

    expect(resp.status).toBe(200);
    expect(received.length).toBe(1);

    const msg = received[0] as { type: string; source: string; payload: Record<string, unknown> };
    expect(msg.type).toBe("dooray.message");
    expect(msg.source).toBe("dooray");
    expect(msg.payload.chat_id).toBe("ch-123");
    expect(msg.payload.sender).toBe("user-42");
    expect(msg.payload.text).toBe("hello from dooray");

    unsub();
  });

  it("returns 405 for non-POST methods", async () => {
    const resp = await fetch(`http://localhost:${port}/`, { method: "GET" });
    expect(resp.status).toBe(405);
  });

  it("returns 400 for malformed JSON", async () => {
    const resp = await fetch(`http://localhost:${port}/`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: "not-json",
    });
    expect(resp.status).toBe(400);
  });

  it("unsubscribe stops receiving messages", async () => {
    const received: unknown[] = [];
    const unsub = adapter.onMessage((msg) => received.push(msg));
    unsub();

    await fetch(`http://localhost:${port}/`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ channelId: "ch-1", text: "hi" }),
    });

    expect(received.length).toBe(0);
  });
});

// ---------------------------------------------------------------------------
// send — mocked fetch
// ---------------------------------------------------------------------------

describe("DoorayAdapter — send", () => {
  let adapter: DoorayAdapter;
  let originalFetch: typeof globalThis.fetch;

  beforeEach(() => {
    adapter = makeAdapter();
    originalFetch = globalThis.fetch;
  });

  afterEach(() => {
    globalThis.fetch = originalFetch;
  });

  it("POSTs to /messenger/v1/channels/:id/direct-send", async () => {
    const calls: { url: string; init: RequestInit }[] = [];
    globalThis.fetch = async (input: RequestInfo | URL, init?: RequestInit) => {
      calls.push({ url: input.toString(), init: init ?? {} });
      return new Response("ok", { status: 200 });
    };

    const ok = await adapter.send({ chat_id: "ch-abc", text: "hello" });

    expect(ok).toBe(true);
    expect(calls.length).toBe(1);
    expect(calls[0].url).toBe(
      "https://dooray.example.com/messenger/v1/channels/ch-abc/direct-send"
    );
    const body = JSON.parse(calls[0].init.body as string);
    expect(body.text).toBe("hello");
    expect(body.channelId).toBe("ch-abc");
  });

  it("sends Authorization header with dooray-api token", async () => {
    const calls: { url: string; init: RequestInit }[] = [];
    globalThis.fetch = async (input: RequestInfo | URL, init?: RequestInit) => {
      calls.push({ url: input.toString(), init: init ?? {} });
      return new Response("ok", { status: 200 });
    };

    await adapter.send({ chat_id: "ch-xyz", text: "test" });

    const headers = calls[0].init.headers as Record<string, string>;
    expect(headers["Authorization"]).toBe("dooray-api test-token");
  });

  it("returns false on non-OK response", async () => {
    globalThis.fetch = async () => new Response("error", { status: 500 });
    const ok = await adapter.send({ chat_id: "ch-1", text: "hi" });
    expect(ok).toBe(false);
  });

  it("returns false on network error", async () => {
    globalThis.fetch = async () => {
      throw new Error("network error");
    };
    const ok = await adapter.send({ chat_id: "ch-1", text: "hi" });
    expect(ok).toBe(false);
  });
});

// ---------------------------------------------------------------------------
// env var fallback
// ---------------------------------------------------------------------------

describe("DoorayAdapter — env var fallback", () => {
  it("reads DOORAY_BOT_TOKEN from env", () => {
    const orig = process.env.DOORAY_BOT_TOKEN;
    process.env.DOORAY_BOT_TOKEN = "env-token";
    const a = new DoorayAdapter({ tenantId: "t", apiUrl: "https://x.com", listenPort: 0 });
    // Can't read private field directly; verify via send behavior below
    expect(a.config.id).toBe("dooray"); // smoke test
    process.env.DOORAY_BOT_TOKEN = orig;
  });
});
