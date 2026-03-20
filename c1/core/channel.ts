/**
 * c1/core/channel.ts
 * MCP Channel server backed by a PlatformAdapter.
 *
 * - Declares the claude/channel capability so Claude Code registers a listener
 * - Exposes a `reply` tool for two-way chat (ListTools + CallTool handlers)
 * - Bridges adapter.onMessage events → mcp.notification (notifications/claude/channel)
 * - Generates an instructions string for Claude's system prompt
 */

import { Server } from "@modelcontextprotocol/sdk/server/index.js";
import {
  ListToolsRequestSchema,
  CallToolRequestSchema,
} from "@modelcontextprotocol/sdk/types.js";
import { type PlatformAdapter, type InboundMessage } from "./adapter.js";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export interface ChannelOptions {
  /** MCP server name (used as `source` attribute in <channel> tags) */
  name: string;
  /** MCP server version */
  version?: string;
  /**
   * Override the default instructions string injected into Claude's system
   * prompt. When omitted a sensible default is generated from `name`.
   */
  instructions?: string;
}

export interface ChannelServer {
  /** The underlying MCP Server instance */
  mcp: Server;
  /**
   * Start bridging adapter messages to MCP notifications.
   * Returns an unsubscribe function that removes the adapter listener.
   */
  connect(adapter: PlatformAdapter): () => void;
  /**
   * Generate the instructions string for the given server name.
   * Exported separately so callers can inspect it without a full server.
   */
}

// ---------------------------------------------------------------------------
// Instructions generator
// ---------------------------------------------------------------------------

/**
 * Build the default instructions string for a channel server.
 * Tells Claude what events to expect and how to use the reply tool.
 */
export function buildInstructions(name: string): string {
  return (
    `Messages from the '${name}' channel arrive as ` +
    `<channel source="${name}" sender="..." chat_id="...">. ` +
    `When a reply is appropriate, call the 'reply' tool with the chat_id ` +
    `from the tag and your response text.`
  );
}

// ---------------------------------------------------------------------------
// createChannelServer factory
// ---------------------------------------------------------------------------

/**
 * Create a configured MCP Channel server.
 *
 * Usage:
 *   const { mcp, connect } = createChannelServer({ name: 'my-channel' })
 *   await mcp.connect(new StdioServerTransport())
 *   const unsub = connect(adapter)   // starts bridging messages
 *   // later: unsub()                // stop bridging
 */
export function createChannelServer(options: ChannelOptions): ChannelServer {
  const { name, version = "0.1.0" } = options;
  const instructions = options.instructions ?? buildInstructions(name);

  // -------------------------------------------------------------------------
  // MCP Server with claude/channel capability
  // -------------------------------------------------------------------------
  const mcp = new Server(
    { name, version },
    {
      capabilities: {
        experimental: { "claude/channel": {} },
        tools: {},
      },
      instructions,
    }
  );

  // -------------------------------------------------------------------------
  // reply tool — ListTools handler
  // -------------------------------------------------------------------------
  mcp.setRequestHandler(ListToolsRequestSchema, async () => ({
    tools: [
      {
        name: "reply",
        description: "Send a message back to the originating chat platform",
        inputSchema: {
          type: "object" as const,
          properties: {
            chat_id: {
              type: "string",
              description: "The conversation / channel ID to reply in",
            },
            text: {
              type: "string",
              description: "The reply text to send",
            },
          },
          required: ["chat_id", "text"],
        },
      },
    ],
  }));

  // -------------------------------------------------------------------------
  // reply tool — CallTool handler
  // -------------------------------------------------------------------------
  // The actual send is delegated to the adapter that is currently bridged.
  // We capture a mutable reference so it can be updated by connect().
  let activeAdapter: PlatformAdapter | null = null;

  mcp.setRequestHandler(CallToolRequestSchema, async (req) => {
    if (req.params.name === "reply") {
      const args = req.params.arguments as { chat_id: string; text: string };
      const { chat_id, text } = args;

      if (!activeAdapter) {
        return {
          content: [{ type: "text" as const, text: "error: no adapter connected" }],
          isError: true,
        };
      }

      const ok = await activeAdapter.send({ chat_id, text });
      return {
        content: [
          {
            type: "text" as const,
            text: ok ? "sent" : "error: adapter send failed",
          },
        ],
        isError: !ok,
      };
    }

    throw new Error(`unknown tool: ${req.params.name}`);
  });

  // -------------------------------------------------------------------------
  // connect — bridge adapter.onMessage → mcp.notification
  // -------------------------------------------------------------------------
  function connect(adapter: PlatformAdapter): () => void {
    activeAdapter = adapter;

    const unsub = adapter.onMessage(async (msg: InboundMessage) => {
      const meta: Record<string, string> = {};

      // Promote well-known scalar fields from the payload into meta attributes.
      // Anything else is serialised into content.
      const payload = msg.payload as Record<string, unknown> | null;
      if (payload && typeof payload === "object") {
        if (typeof payload.chat_id === "string") meta.chat_id = payload.chat_id;
        if (typeof payload.sender === "string") meta.sender = payload.sender;
        if (typeof payload.thread_id === "string")
          meta.thread_id = payload.thread_id;
      }

      // The `type` field from InboundMessage is safe to include as meta
      if (msg.type) meta.type = msg.type;
      // source becomes the channel `source` attribute automatically from server name,
      // but expose it explicitly as well for traceability
      meta.adapter = msg.source;

      const content =
        typeof payload?.text === "string"
          ? payload.text
          : JSON.stringify(msg.payload);

      await mcp.notification({
        method: "notifications/claude/channel",
        params: { content, meta },
      });
    });

    return () => {
      unsub();
      if (activeAdapter === adapter) activeAdapter = null;
    };
  }

  return { mcp, connect };
}
