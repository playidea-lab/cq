/**
 * c1/core/adapter.ts
 * PlatformAdapter interface, AdapterConfig/InboundMessage types, AdapterRegistry.
 */

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export interface AdapterConfig {
  /** Unique adapter identifier (e.g. "tauri", "electron", "web") */
  id: string;
  /** Human-readable name */
  name: string;
  /** Arbitrary platform-specific options */
  options?: Record<string, unknown>;
}

export interface InboundMessage {
  /** Message type discriminator */
  type: string;
  /** Originating adapter id */
  source: string;
  /** Unix epoch ms */
  timestamp: number;
  /** Message payload */
  payload: unknown;
}

// ---------------------------------------------------------------------------
// PlatformAdapter interface
// ---------------------------------------------------------------------------

export interface PlatformAdapter {
  /** Configuration supplied during registration */
  readonly config: AdapterConfig;

  /**
   * Initialize the adapter (open connections, load resources, etc.)
   * Must be idempotent.
   */
  initialize(): Promise<void>;

  /**
   * Destroy the adapter (close connections, free resources, etc.)
   * Must be idempotent.
   */
  destroy(): Promise<void>;

  /**
   * Send a message to the platform.
   * Returns true on success.
   */
  send(message: unknown): Promise<boolean>;

  /**
   * Register a handler for inbound messages.
   * Returns an unsubscribe function.
   */
  onMessage(handler: (msg: InboundMessage) => void): () => void;
}

// ---------------------------------------------------------------------------
// AdapterRegistry
// ---------------------------------------------------------------------------

export class AdapterRegistry {
  private readonly adapters = new Map<string, PlatformAdapter>();

  /**
   * Register an adapter. Throws if an adapter with the same id already exists.
   */
  register(adapter: PlatformAdapter): void {
    const { id } = adapter.config;
    if (this.adapters.has(id)) {
      throw new Error(`Adapter '${id}' is already registered`);
    }
    this.adapters.set(id, adapter);
  }

  /**
   * Unregister an adapter by id.
   * Returns true if the adapter was found and removed.
   */
  unregister(id: string): boolean {
    return this.adapters.delete(id);
  }

  /**
   * Retrieve a registered adapter by id.
   * Returns undefined if not found.
   */
  get(id: string): PlatformAdapter | undefined {
    return this.adapters.get(id);
  }

  /**
   * Return all registered adapter ids.
   */
  ids(): string[] {
    return [...this.adapters.keys()];
  }

  /**
   * Return the number of registered adapters.
   */
  size(): number {
    return this.adapters.size;
  }

  /**
   * Initialize all registered adapters in parallel.
   */
  async initializeAll(): Promise<void> {
    await Promise.all([...this.adapters.values()].map((a) => a.initialize()));
  }

  /**
   * Destroy all registered adapters in parallel.
   */
  async destroyAll(): Promise<void> {
    await Promise.all([...this.adapters.values()].map((a) => a.destroy()));
  }
}
