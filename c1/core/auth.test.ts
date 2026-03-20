/**
 * c1/core/auth.test.ts
 * Tests for AuthManager: allowlist, pairing code gen/verify, sender gating.
 */
import { describe, it, expect, beforeEach } from "bun:test";
import { writeFileSync, unlinkSync } from "fs";
import { tmpdir } from "os";
import { join } from "path";
import { AuthManager, type AccessConfig } from "./auth";

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function makeAuth(overrides?: Partial<AccessConfig>): AuthManager {
  return new AuthManager({
    allowlist: ["alice", "bob"],
    pairing_ttl_sec: 60,
    ...overrides,
  });
}

// ---------------------------------------------------------------------------
// Allowlist tests
// ---------------------------------------------------------------------------

describe("AuthManager – allowlist", () => {
  let auth: AuthManager;
  beforeEach(() => {
    auth = makeAuth();
  });

  it("allows listed senders", () => {
    expect(auth.isAllowed("alice")).toBe(true);
    expect(auth.isAllowed("bob")).toBe(true);
  });

  it("denies unlisted senders", () => {
    expect(auth.isAllowed("eve")).toBe(false);
  });

  it("addSender makes a new sender allowed", () => {
    auth.addSender("charlie");
    expect(auth.isAllowed("charlie")).toBe(true);
  });

  it("removeSender removes an existing sender", () => {
    auth.removeSender("alice");
    expect(auth.isAllowed("alice")).toBe(false);
  });

  it("removeSender is a no-op for unknown sender", () => {
    expect(() => auth.removeSender("nobody")).not.toThrow();
  });

  it("getAllowlist returns current members", () => {
    const list = auth.getAllowlist();
    expect(list.sort()).toEqual(["alice", "bob"]);
  });

  it("getAllowlist reflects runtime changes", () => {
    auth.addSender("charlie");
    auth.removeSender("bob");
    expect(auth.getAllowlist().sort()).toEqual(["alice", "charlie"]);
  });
});

// ---------------------------------------------------------------------------
// Sender gating tests
// ---------------------------------------------------------------------------

describe("AuthManager – sender gating", () => {
  let auth: AuthManager;
  beforeEach(() => {
    auth = makeAuth();
  });

  it("assertAllowed does not throw for allowed sender", () => {
    expect(() => auth.assertAllowed("alice")).not.toThrow();
  });

  it("assertAllowed throws for disallowed sender", () => {
    expect(() => auth.assertAllowed("eve")).toThrow(
      "Sender 'eve' is not in the allowlist"
    );
  });
});

// ---------------------------------------------------------------------------
// Pairing code tests
// ---------------------------------------------------------------------------

describe("AuthManager – pairing code", () => {
  let auth: AuthManager;
  beforeEach(() => {
    auth = makeAuth({ allowlist: [], pairing_ttl_sec: 5 });
  });

  it("generateCode returns a 6-char uppercase hex string", () => {
    const code = auth.generateCode("newuser");
    expect(code).toMatch(/^[0-9A-F]{6}$/);
  });

  it("verifyCode with correct code adds sender to allowlist", () => {
    const code = auth.generateCode("newuser");
    const ok = auth.verifyCode("newuser", code);
    expect(ok).toBe(true);
    expect(auth.isAllowed("newuser")).toBe(true);
  });

  it("verifyCode removes pending code on success", () => {
    const code = auth.generateCode("newuser");
    auth.verifyCode("newuser", code);
    expect(auth.pendingCount()).toBe(0);
  });

  it("verifyCode returns false for wrong code", () => {
    auth.generateCode("newuser");
    const ok = auth.verifyCode("newuser", "ZZZZZZ");
    expect(ok).toBe(false);
    expect(auth.isAllowed("newuser")).toBe(false);
  });

  it("verifyCode returns false when no code exists", () => {
    const ok = auth.verifyCode("ghost", "AABBCC");
    expect(ok).toBe(false);
  });

  it("verifyCode returns false for expired code", async () => {
    // Use a 0-second TTL by creating a custom auth
    const shortAuth = new AuthManager({ allowlist: [], pairing_ttl_sec: 0 });
    const code = shortAuth.generateCode("newuser");
    // Immediately expired (TTL=0 means expires_at = now+0ms, already past)
    await new Promise((r) => setTimeout(r, 5));
    const ok = shortAuth.verifyCode("newuser", code);
    expect(ok).toBe(false);
    expect(shortAuth.isAllowed("newuser")).toBe(false);
  });

  it("generateCode overwrites previous code for same sender", () => {
    const first = auth.generateCode("newuser");
    const second = auth.generateCode("newuser");
    // second code should be valid; first is superseded (may or may not be same random value, but verify second works)
    expect(auth.verifyCode("newuser", second)).toBe(true);
    expect(auth.pendingCount()).toBe(0);
  });

  it("pruneExpiredCodes removes expired entries", async () => {
    const shortAuth = new AuthManager({ allowlist: [], pairing_ttl_sec: 0 });
    shortAuth.generateCode("user1");
    shortAuth.generateCode("user2");
    await new Promise((r) => setTimeout(r, 5));
    shortAuth.pruneExpiredCodes();
    expect(shortAuth.pendingCount()).toBe(0);
  });

  it("pendingCount increments per generateCode", () => {
    auth.generateCode("u1");
    auth.generateCode("u2");
    expect(auth.pendingCount()).toBe(2);
  });
});

// ---------------------------------------------------------------------------
// fromFile tests
// ---------------------------------------------------------------------------

describe("AuthManager.fromFile", () => {
  it("loads a valid access.json", () => {
    const path = join(tmpdir(), `access-${Date.now()}.json`);
    const config: AccessConfig = { allowlist: ["alice"], pairing_ttl_sec: 120 };
    writeFileSync(path, JSON.stringify(config), "utf-8");

    const auth = AuthManager.fromFile(path);
    expect(auth.isAllowed("alice")).toBe(true);
    expect(auth.isAllowed("bob")).toBe(false);

    unlinkSync(path);
  });

  it("throws when file does not exist", () => {
    expect(() => AuthManager.fromFile("/tmp/nonexistent-access.json")).toThrow(
      "access.json not found"
    );
  });

  it("throws when JSON is malformed", () => {
    const path = join(tmpdir(), `access-bad-${Date.now()}.json`);
    writeFileSync(path, "{ not valid json }", "utf-8");
    expect(() => AuthManager.fromFile(path)).toThrow("Failed to parse access.json");
    unlinkSync(path);
  });

  it("throws when allowlist key is missing", () => {
    const path = join(tmpdir(), `access-no-list-${Date.now()}.json`);
    writeFileSync(path, JSON.stringify({ other: "data" }), "utf-8");
    expect(() => AuthManager.fromFile(path)).toThrow(
      "access.json must have an 'allowlist' array"
    );
    unlinkSync(path);
  });
});
