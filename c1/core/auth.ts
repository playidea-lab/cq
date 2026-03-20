/**
 * c1/core/auth.ts
 * Allowlist-based sender gating + pairing code generation/verification.
 *
 * access.json format:
 * {
 *   "allowlist": ["user-id-1", "user-id-2"],
 *   "pairing_ttl_sec": 300
 * }
 */

import { readFileSync, existsSync } from "fs";
import { randomBytes } from "crypto";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export interface AccessConfig {
  /** List of allowed sender IDs */
  allowlist: string[];
  /** Pairing code TTL in seconds (default: 300) */
  pairing_ttl_sec?: number;
}

export interface PairingCode {
  code: string;
  expires_at: number; // Unix epoch ms
}

// ---------------------------------------------------------------------------
// AuthManager
// ---------------------------------------------------------------------------

export class AuthManager {
  private allowlist: Set<string>;
  private pairingTtlMs: number;
  private pendingCodes: Map<string, PairingCode> = new Map();

  constructor(config: AccessConfig) {
    this.allowlist = new Set(config.allowlist);
    this.pairingTtlMs = (config.pairing_ttl_sec ?? 300) * 1000;
  }

  /**
   * Load an AuthManager from an access.json file path.
   * Throws if the file is missing or malformed.
   */
  static fromFile(path: string): AuthManager {
    if (!existsSync(path)) {
      throw new Error(`access.json not found: ${path}`);
    }
    const raw = readFileSync(path, "utf-8");
    let config: AccessConfig;
    try {
      config = JSON.parse(raw) as AccessConfig;
    } catch (e) {
      throw new Error(`Failed to parse access.json: ${(e as Error).message}`);
    }
    if (!Array.isArray(config.allowlist)) {
      throw new Error("access.json must have an 'allowlist' array");
    }
    return new AuthManager(config);
  }

  // ---------------------------------------------------------------------------
  // Allowlist API
  // ---------------------------------------------------------------------------

  /**
   * Returns true if the sender is in the allowlist.
   */
  isAllowed(senderId: string): boolean {
    return this.allowlist.has(senderId);
  }

  /**
   * Add a sender to the allowlist at runtime.
   */
  addSender(senderId: string): void {
    this.allowlist.add(senderId);
  }

  /**
   * Remove a sender from the allowlist at runtime.
   */
  removeSender(senderId: string): void {
    this.allowlist.delete(senderId);
  }

  /**
   * Return a snapshot of the current allowlist.
   */
  getAllowlist(): string[] {
    return [...this.allowlist];
  }

  // ---------------------------------------------------------------------------
  // Sender gating API
  // ---------------------------------------------------------------------------

  /**
   * Throw if the sender is not allowed.
   * Use this to guard message handlers.
   */
  assertAllowed(senderId: string): void {
    if (!this.isAllowed(senderId)) {
      throw new Error(`Sender '${senderId}' is not in the allowlist`);
    }
  }

  // ---------------------------------------------------------------------------
  // Pairing code API
  // ---------------------------------------------------------------------------

  /**
   * Generate a new pairing code for a pending sender.
   * Overwrites any existing code for the same senderId.
   * Returns the pairing code (6 uppercase hex chars).
   */
  generateCode(senderId: string): string {
    const code = randomBytes(3).toString("hex").toUpperCase();
    this.pendingCodes.set(senderId, {
      code,
      expires_at: Date.now() + this.pairingTtlMs,
    });
    return code;
  }

  /**
   * Verify a pairing code for the given sender.
   * On success: adds sender to the allowlist and removes the pending code.
   * Returns true on success, false on wrong code or expired.
   */
  verifyCode(senderId: string, code: string): boolean {
    const pending = this.pendingCodes.get(senderId);
    if (!pending) return false;
    if (Date.now() > pending.expires_at) {
      this.pendingCodes.delete(senderId);
      return false;
    }
    if (pending.code !== code) return false;

    // Promote sender to allowlist
    this.allowlist.add(senderId);
    this.pendingCodes.delete(senderId);
    return true;
  }

  /**
   * Remove all expired pairing codes.
   */
  pruneExpiredCodes(): void {
    const now = Date.now();
    for (const [id, entry] of this.pendingCodes) {
      if (now > entry.expires_at) {
        this.pendingCodes.delete(id);
      }
    }
  }

  /**
   * Return the number of pending (not yet verified) pairing codes.
   */
  pendingCount(): number {
    return this.pendingCodes.size;
  }
}
