/**
 * c1/core/channel.test.ts
 * Unit tests for the MCP Channel server (channel.ts).
 */
import { describe, it, expect, mock, beforeEach, spyOn } from "bun:test";
import { buildInstructions, createChannelServer } from "./channel.js";
import type { PlatformAdapter, InboundMessage } from "./adapter.js";

// ---------------------------------------------------------------------------
// Minimal PlatformAdapter stub
// ---------------------------------------------------------------------------

interface MessageHandler {
  (msg: InboundMessage): void;
}

function makeAdapter(id: string): PlatformAdapter & { _emit: (msg: InboundMessage) => void } {
  const handlers: MessageHandler[] = [];
  const send = mock(async (_msg: unknown) => true);

  return {
    config: { id, name: `Adapter-${id}` },
    initialize: mock(async () => {}),
    destroy: mock(async () => {}),
    send,
    onMessage(handler: MessageHandler) {
      handlers.push(handler);
      return () => {
        const i = handlers.indexOf(handler);
        if (i !== -1) handlers.splice(i, 1);
      };
    },
    _emit(msg: InboundMessage) {
      handlers.forEach((h) => h(msg));
    },
  };
}

// ---------------------------------------------------------------------------
// Helper: invoke stored request handler by method key with a full request
// ---------------------------------------------------------------------------
async function invokeHandler(
  mcp: ReturnType<typeof createChannelServer>["mcp"],
  method: string,
  params: unknown = {}
) {
  const handler = (mcp as any)._requestHandlers?.get(method);
  if (!handler) throw new Error(`No handler for method: ${method}`);
  return handler({ method, params }, {});
}

// ---------------------------------------------------------------------------
// buildInstructions
// ---------------------------------------------------------------------------

describe("buildInstructions", () => {
  it("includes the server name", () => {
    expect(buildInstructions("dooray")).toContain("dooray");
  });

  it("mentions the reply tool", () => {
    expect(buildInstructions("slack")).toContain("reply");
  });

  it("mentions chat_id", () => {
    expect(buildInstructions("teams")).toContain("chat_id");
  });

  it("uses the name as the source attribute", () => {
    expect(buildInstructions("test-ch")).toContain('source="test-ch"');
  });
});

// ---------------------------------------------------------------------------
// createChannelServer — server configuration
// ---------------------------------------------------------------------------

describe("createChannelServer — server config", () => {
  it("returns an mcp Server and connect function", () => {
    const { mcp, connect } = createChannelServer({ name: "test" });
    expect(mcp).toBeDefined();
    expect(typeof connect).toBe("function");
  });

  it("accepts a custom instructions override without throwing", () => {
    const { mcp } = createChannelServer({
      name: "test",
      instructions: "custom instructions string",
    });
    expect(mcp).toBeDefined();
  });
});

// ---------------------------------------------------------------------------
// ListTools handler
// ---------------------------------------------------------------------------

describe("createChannelServer — ListTools", () => {
  it("returns a tool named 'reply'", async () => {
    const { mcp } = createChannelServer({ name: "ch" });
    const result = await invokeHandler(mcp, "tools/list");
    expect(result.tools).toBeDefined();
    const names: string[] = result.tools.map((t: { name: string }) => t.name);
    expect(names).toContain("reply");
  });

  it("reply tool has chat_id and text in inputSchema", async () => {
    const { mcp } = createChannelServer({ name: "ch" });
    const result = await invokeHandler(mcp, "tools/list");
    const reply = result.tools.find((t: { name: string }) => t.name === "reply");
    expect(reply).toBeDefined();
    expect(reply.inputSchema.properties.chat_id).toBeDefined();
    expect(reply.inputSchema.properties.text).toBeDefined();
  });
});

// ---------------------------------------------------------------------------
// CallTool — reply handler
// ---------------------------------------------------------------------------

describe("createChannelServer — CallTool reply", () => {
  let adapter: ReturnType<typeof makeAdapter>;

  beforeEach(() => {
    adapter = makeAdapter("test-adapter");
  });

  it("delegates send to the active adapter", async () => {
    const { mcp, connect } = createChannelServer({ name: "ch" });
    connect(adapter);

    const result = await invokeHandler(mcp, "tools/call", {
      name: "reply",
      arguments: { chat_id: "c1", text: "hi" },
    });
    expect(adapter.send).toHaveBeenCalledWith({ chat_id: "c1", text: "hi" });
    expect(result.content[0].text).toBe("sent");
    expect(result.isError).toBeFalsy();
  });

  it("returns error content when no adapter is connected", async () => {
    const { mcp } = createChannelServer({ name: "ch" });
    // No connect() called

    const result = await invokeHandler(mcp, "tools/call", {
      name: "reply",
      arguments: { chat_id: "c1", text: "hi" },
    });
    expect(result.isError).toBe(true);
  });

  it("throws on unknown tool name", async () => {
    const { mcp } = createChannelServer({ name: "ch" });
    await expect(
      invokeHandler(mcp, "tools/call", {
        name: "nonexistent",
        arguments: {},
      })
    ).rejects.toThrow("unknown tool: nonexistent");
  });
});

// ---------------------------------------------------------------------------
// connect — adapter bridging
// ---------------------------------------------------------------------------

describe("createChannelServer — connect", () => {
  let adapter: ReturnType<typeof makeAdapter>;

  beforeEach(() => {
    adapter = makeAdapter("test-adapter");
  });

  it("subscribes to adapter messages on connect", () => {
    const { connect } = createChannelServer({ name: "ch" });
    const spy = spyOn(adapter, "onMessage");
    connect(adapter);
    expect(spy).toHaveBeenCalledTimes(1);
  });

  it("returns an unsubscribe function", () => {
    const { connect } = createChannelServer({ name: "ch" });
    const unsub = connect(adapter);
    expect(typeof unsub).toBe("function");
    expect(() => unsub()).not.toThrow();
  });

  it("unsubscribe stops message forwarding", async () => {
    const { mcp, connect } = createChannelServer({ name: "ch" });

    const captured: unknown[] = [];
    spyOn(mcp, "notification").mockImplementation(async (n) => {
      captured.push(n);
    });

    const unsub = connect(adapter);

    adapter._emit({
      type: "chat",
      source: "test-adapter",
      timestamp: Date.now(),
      payload: { text: "hello" },
    });
    await Promise.resolve();
    expect(captured.length).toBe(1);

    unsub();
    adapter._emit({
      type: "chat",
      source: "test-adapter",
      timestamp: Date.now(),
      payload: { text: "after unsub" },
    });
    await Promise.resolve();
    expect(captured.length).toBe(1); // unchanged
  });

  it("forwards inbound message text as notification content", async () => {
    const { mcp, connect } = createChannelServer({ name: "ch" });

    const captured: Array<{ method: string; params: Record<string, unknown> }> = [];
    spyOn(mcp, "notification").mockImplementation(async (n) => {
      captured.push(n as typeof captured[0]);
    });

    connect(adapter);
    adapter._emit({
      type: "chat",
      source: "test-adapter",
      timestamp: Date.now(),
      payload: { text: "world", chat_id: "room-1", sender: "user-42" },
    });
    await Promise.resolve();

    expect(captured).toHaveLength(1);
    const { method, params } = captured[0];
    expect(method).toBe("notifications/claude/channel");
    expect((params as any).content).toBe("world");
    expect((params as any).meta?.chat_id).toBe("room-1");
    expect((params as any).meta?.sender).toBe("user-42");
    expect((params as any).meta?.adapter).toBe("test-adapter");
  });

  it("serialises non-text payload to JSON string", async () => {
    const { mcp, connect } = createChannelServer({ name: "ch" });

    const captured: Array<{ method: string; params: Record<string, unknown> }> = [];
    spyOn(mcp, "notification").mockImplementation(async (n) => {
      captured.push(n as typeof captured[0]);
    });

    connect(adapter);
    const payload = { event: "build_failed", run_id: 99 };
    adapter._emit({
      type: "ci",
      source: "test-adapter",
      timestamp: Date.now(),
      payload,
    });
    await Promise.resolve();

    expect(captured).toHaveLength(1);
    const content = (captured[0].params as any).content;
    expect(content).toBe(JSON.stringify(payload));
  });

  it("sets type meta from inbound message type", async () => {
    const { mcp, connect } = createChannelServer({ name: "ch" });

    const captured: Array<{ params: Record<string, unknown> }> = [];
    spyOn(mcp, "notification").mockImplementation(async (n) => {
      captured.push(n as typeof captured[0]);
    });

    connect(adapter);
    adapter._emit({
      type: "webhook",
      source: "test-adapter",
      timestamp: Date.now(),
      payload: { text: "ping" },
    });
    await Promise.resolve();

    expect((captured[0].params as any).meta?.type).toBe("webhook");
  });
});
