/**
 * c1/adapters/dooray/dooray.test.ts
 * Unit tests for DoorayAdapter (Hub polling mode) using bun:test.
 */

import { describe, test, expect, beforeEach, afterEach } from "bun:test";
import { DoorayAdapter } from "./index.js";
import type { InboundMessage } from "../../core/adapter.js";

// ---------------------------------------------------------------------------
// Mock Hub server
// ---------------------------------------------------------------------------

let mockServer: ReturnType<typeof Bun.serve> | null = null;
let pendingMessages: any[] = [];
let lastReply: any = null;

function startMockHub(port: number) {
  pendingMessages = [];
  lastReply = null;
  mockServer = Bun.serve({
    port,
    hostname: "127.0.0.1",
    fetch: async (req) => {
      const url = new URL(req.url);

      if (url.pathname === "/v1/dooray/pending" && req.method === "GET") {
        const msgs = [...pendingMessages];
        pendingMessages = [];
        return Response.json(msgs);
      }

      if (url.pathname === "/v1/dooray/reply" && req.method === "POST") {
        lastReply = await req.json();
        return new Response("ok", { status: 200 });
      }

      return new Response("not found", { status: 404 });
    },
  });
}

function stopMockHub() {
  mockServer?.stop();
  mockServer = null;
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

const TEST_PORT = 19981;

describe("DoorayAdapter (Hub polling)", () => {
  let adapter: DoorayAdapter;

  beforeEach(() => {
    startMockHub(TEST_PORT);
    adapter = new DoorayAdapter({
      hubUrl: `http://127.0.0.1:${TEST_PORT}`,
      pollIntervalMs: 100,
    });
  });

  afterEach(async () => {
    await adapter.destroy();
    stopMockHub();
  });

  test("config has correct defaults", () => {
    expect(adapter.config.id).toBe("dooray");
    expect(adapter.config.name).toBe("Dooray Messenger (Hub polling)");
  });

  test("initialize starts polling (idempotent)", async () => {
    await adapter.initialize();
    await adapter.initialize();
  });

  test("destroy stops polling (idempotent)", async () => {
    await adapter.initialize();
    await adapter.destroy();
    await adapter.destroy();
  });

  test("polls pending messages and dispatches to handler", async () => {
    const received: InboundMessage[] = [];
    adapter.onMessage((msg) => received.push(msg));

    pendingMessages.push({
      channelId: "ch-1",
      senderId: "alice",
      text: "hello from dooray",
      response_url: "https://x.dooray.com/resp/1",
    });

    await adapter.initialize();
    await new Promise((r) => setTimeout(r, 300));

    expect(received.length).toBe(1);
    expect(received[0].payload.text).toBe("hello from dooray");
    expect(received[0].payload.chat_id).toBe("ch-1");
    expect(received[0].payload.response_url).toBe("https://x.dooray.com/resp/1");
    expect(received[0].source).toBe("dooray");
  });

  test("polls multiple messages in one batch", async () => {
    const received: InboundMessage[] = [];
    adapter.onMessage((msg) => received.push(msg));

    pendingMessages.push(
      { channelId: "ch-1", senderId: "a", text: "msg1" },
      { channelId: "ch-2", senderId: "b", text: "msg2" },
    );

    await adapter.initialize();
    await new Promise((r) => setTimeout(r, 300));

    expect(received.length).toBe(2);
    expect(received[0].payload.text).toBe("msg1");
    expect(received[1].payload.text).toBe("msg2");
  });

  test("empty pending returns no messages", async () => {
    const received: InboundMessage[] = [];
    adapter.onMessage((msg) => received.push(msg));

    await adapter.initialize();
    await new Promise((r) => setTimeout(r, 300));

    expect(received.length).toBe(0);
  });

  test("send via Hub reply proxy", async () => {
    const ok = await adapter.send({
      chat_id: "ch-1",
      text: "reply text",
      response_url: "https://x.dooray.com/resp/1",
    });

    expect(ok).toBe(true);
    expect(lastReply).toEqual({
      response_url: "https://x.dooray.com/resp/1",
      text: "reply text",
      response_type: "inChannel",
    });
  });

  test("send without response_url returns false", async () => {
    const ok = await adapter.send({
      chat_id: "ch-1",
      text: "no url",
    });
    expect(ok).toBe(false);
  });

  test("unsubscribe handler", async () => {
    const received: InboundMessage[] = [];
    const unsub = adapter.onMessage((msg) => received.push(msg));
    unsub();

    pendingMessages.push({ channelId: "ch-1", text: "ignored" });
    await adapter.initialize();
    await new Promise((r) => setTimeout(r, 300));

    expect(received.length).toBe(0);
  });

  test("handles Hub server down gracefully", async () => {
    stopMockHub();
    await adapter.initialize();
    await new Promise((r) => setTimeout(r, 300));
    // no error = graceful failure
  });
});
