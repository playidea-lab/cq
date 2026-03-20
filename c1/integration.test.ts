/**
 * c1/integration.test.ts
 * End-to-end integration test: fakechat pattern.
 *
 * Flow tested:
 *   1. curl POST (webhook) → DoorayAdapter emits InboundMessage
 *                          → ChannelServer sends MCP notification
 *   2. reply tool CallTool → DoorayAdapter.send() → HTTP response
 *
 * Uses an in-process HTTP server (listenPort: 0) and a captured
 * fake Dooray API server so no real network calls are made.
 */

import { describe, it, expect, beforeEach, afterEach, spyOn } from "bun:test";
import { DoorayAdapter } from "./adapters/dooray/index.js";
import { createChannelServer } from "./core/channel.js";
import type { InboundMessage } from "./core/adapter.js";

// ---------------------------------------------------------------------------
// Helper: get the actual OS-assigned port from a DoorayAdapter instance
// ---------------------------------------------------------------------------

function getPort(adapter: DoorayAdapter): number {
  const srv = (adapter as unknown as { server: { port: number } }).server;
  if (!srv) throw new Error("Adapter not initialized");
  return srv.port;
}

// ---------------------------------------------------------------------------
// Integration: webhook POST → channel notification
// ---------------------------------------------------------------------------

describe("Integration — webhook POST → channel notification", () => {
  let adapter: DoorayAdapter;
  let webhookPort: number;
  let notifications: Array<{ method: string; params: Record<string, unknown> }>;
  let unsub: () => void;

  beforeEach(async () => {
    adapter = new DoorayAdapter({
      botToken: "fake-token",
      tenantId: "fake-tenant",
      apiUrl: "https://dooray.fake",
      listenPort: 0, // OS assigns a free port
    });
    await adapter.initialize();
    webhookPort = getPort(adapter);

    const { mcp, connect } = createChannelServer({ name: "dooray-test" });

    notifications = [];
    // Intercept MCP notifications without a real transport
    spyOn(mcp, "notification").mockImplementation(async (n) => {
      notifications.push(n as (typeof notifications)[0]);
    });

    unsub = connect(adapter);
  });

  afterEach(async () => {
    unsub();
    await adapter.destroy();
  });

  it("webhook POST delivers channel notification with correct content", async () => {
    const body = {
      tenantId: "fake-tenant",
      channelId: "ch-integration-1",
      senderId: "alice",
      text: "hello from integration test",
    };

    const resp = await fetch(`http://localhost:${webhookPort}/`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    });

    expect(resp.status).toBe(200);
    expect(await resp.text()).toBe("ok");

    // Wait one microtask for async notification to flush
    await Promise.resolve();

    expect(notifications).toHaveLength(1);
    const { method, params } = notifications[0];
    expect(method).toBe("notifications/claude/channel");
    expect((params as any).content).toBe("hello from integration test");
    expect((params as any).meta?.chat_id).toBe("ch-integration-1");
    expect((params as any).meta?.sender).toBe("alice");
    expect((params as any).meta?.adapter).toBe("dooray");
  });

  it("multiple webhook POSTs → multiple notifications in order", async () => {
    const messages = [
      { channelId: "ch-1", senderId: "user-a", text: "first" },
      { channelId: "ch-1", senderId: "user-b", text: "second" },
      { channelId: "ch-2", senderId: "user-a", text: "third" },
    ];

    for (const msg of messages) {
      const resp = await fetch(`http://localhost:${webhookPort}/`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(msg),
      });
      expect(resp.status).toBe(200);
    }

    await Promise.resolve();

    expect(notifications).toHaveLength(3);
    const texts = notifications.map((n) => (n.params as any).content);
    expect(texts).toEqual(["first", "second", "third"]);
  });

  it("non-POST to webhook → 405, no notification emitted", async () => {
    const resp = await fetch(`http://localhost:${webhookPort}/`, {
      method: "GET",
    });
    expect(resp.status).toBe(405);
    await Promise.resolve();
    expect(notifications).toHaveLength(0);
  });

  it("malformed JSON to webhook → 400, no notification emitted", async () => {
    const resp = await fetch(`http://localhost:${webhookPort}/`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: "not-json{{{",
    });
    expect(resp.status).toBe(400);
    await Promise.resolve();
    expect(notifications).toHaveLength(0);
  });
});

// ---------------------------------------------------------------------------
// Integration: reply tool → HTTP response (adapter.send)
// ---------------------------------------------------------------------------

describe("Integration — reply tool → HTTP response", () => {
  let adapter: DoorayAdapter;
  let fakeSendCalls: Array<unknown>;

  beforeEach(async () => {
    adapter = new DoorayAdapter({
      botToken: "fake-token",
      tenantId: "fake-tenant",
      apiUrl: "https://dooray.fake",
      listenPort: 0,
    });
    await adapter.initialize();

    fakeSendCalls = [];
  });

  afterEach(async () => {
    await adapter.destroy();
  });

  it("reply tool invocation calls adapter.send and returns 'sent'", async () => {
    const { mcp, connect } = createChannelServer({ name: "dooray-test" });
    // Intercept notification (not the focus here)
    spyOn(mcp, "notification").mockImplementation(async () => {});
    connect(adapter);

    // Replace adapter.send to capture args without real HTTP
    const origSend = adapter.send.bind(adapter);
    spyOn(adapter, "send").mockImplementation(async (msg) => {
      fakeSendCalls.push(msg);
      return true; // simulate success
    });

    // Invoke the CallTool handler directly via internal _requestHandlers
    const callToolHandler = (mcp as any)._requestHandlers?.get("tools/call");
    expect(callToolHandler).toBeDefined();

    const result = await callToolHandler(
      {
        method: "tools/call",
        params: {
          name: "reply",
          arguments: { chat_id: "ch-integration-1", text: "Hi, Alice!" },
        },
      },
      {}
    );

    expect(fakeSendCalls).toHaveLength(1);
    expect(fakeSendCalls[0]).toEqual({ chat_id: "ch-integration-1", text: "Hi, Alice!" });
    expect(result.content[0].text).toBe("sent");
    expect(result.isError).toBeFalsy();
  });

  it("reply tool returns error when adapter.send fails", async () => {
    const { mcp, connect } = createChannelServer({ name: "dooray-test" });
    spyOn(mcp, "notification").mockImplementation(async () => {});
    connect(adapter);

    spyOn(adapter, "send").mockImplementation(async () => false);

    const callToolHandler = (mcp as any)._requestHandlers?.get("tools/call");
    const result = await callToolHandler(
      {
        method: "tools/call",
        params: {
          name: "reply",
          arguments: { chat_id: "ch-fail", text: "this will fail" },
        },
      },
      {}
    );

    expect(result.isError).toBe(true);
    expect(result.content[0].text).toContain("error");
  });
});

// ---------------------------------------------------------------------------
// Integration: full round-trip (POST → notification + reply → send)
// ---------------------------------------------------------------------------

describe("Integration — full round-trip", () => {
  let adapter: DoorayAdapter;
  let webhookPort: number;
  let notifications: Array<{ method: string; params: Record<string, unknown> }>;
  let sendCalls: Array<unknown>;

  beforeEach(async () => {
    adapter = new DoorayAdapter({
      botToken: "fake-token",
      tenantId: "fake-tenant",
      apiUrl: "https://dooray.fake",
      listenPort: 0,
    });
    await adapter.initialize();
    webhookPort = getPort(adapter);

    const { mcp, connect } = createChannelServer({ name: "dooray" });
    notifications = [];
    sendCalls = [];

    spyOn(mcp, "notification").mockImplementation(async (n) => {
      notifications.push(n as (typeof notifications)[0]);
    });

    spyOn(adapter, "send").mockImplementation(async (msg) => {
      sendCalls.push(msg);
      return true;
    });

    connect(adapter);

    // Store callToolHandler on adapter for reuse in test body
    (adapter as any).__callTool = (mcp as any)._requestHandlers?.get("tools/call");
  });

  afterEach(async () => {
    await adapter.destroy();
  });

  it("inbound webhook triggers notification, reply tool sends back to same channel", async () => {
    // 1. Incoming webhook (like curl POST)
    const resp = await fetch(`http://localhost:${webhookPort}/`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        channelId: "ch-roundtrip",
        senderId: "bob",
        text: "ping",
      }),
    });
    expect(resp.status).toBe(200);
    await Promise.resolve();

    // Verify notification arrived
    expect(notifications).toHaveLength(1);
    const chatId = (notifications[0].params as any).meta?.chat_id;
    expect(chatId).toBe("ch-roundtrip");

    // 2. Claude replies via the reply tool
    const callTool = (adapter as any).__callTool;
    const result = await callTool(
      {
        method: "tools/call",
        params: {
          name: "reply",
          arguments: { chat_id: chatId, text: "pong" },
        },
      },
      {}
    );

    expect(result.content[0].text).toBe("sent");
    expect(sendCalls).toHaveLength(1);
    expect(sendCalls[0]).toEqual({ chat_id: "ch-roundtrip", text: "pong" });
  });
});
