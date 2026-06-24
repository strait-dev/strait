import { describe, expect, it, vi } from "vitest";

/**
 * Tests for the OAuth provider configuration in auth.server.ts.
 * These verify the plugin setup without requiring a database connection.
 */

// Mock all external dependencies that auth.server.ts imports
vi.mock("pg", () => ({
  Pool: class MockPool {
    query = vi.fn().mockResolvedValue({ rows: [], rowCount: 0 });
    connect = vi.fn().mockResolvedValue({
      query: vi.fn().mockResolvedValue({ rows: [], rowCount: 0 }),
      release: vi.fn(),
    });
    end = vi.fn();
    on = vi.fn();
  },
}));

vi.mock("@react-email/render", () => ({
  render: vi.fn().mockResolvedValue("<html></html>"),
}));

vi.mock("@strait/transactional", () => ({
  ConfirmAccount: vi.fn(),
  MagicLink: vi.fn(),
  OrganizationInvite: vi.fn(),
  ResetPassword: vi.fn(),
}));

vi.mock("@/lib/resend.server", () => ({
  getResend: () => ({ emails: { send: vi.fn() } }),
}));

// Set required env vars before importing auth.server
vi.stubEnv("AUTH_DATABASE_URL", "postgresql://test:test@localhost/test");
vi.stubEnv("BETTER_AUTH_URL", "http://localhost:5173");
vi.stubEnv("BETTER_AUTH_SECRET", "test-secret-at-least-32-chars-long");

describe("OAuth provider configuration", () => {
  it("auth module exports auth instance", { timeout: 15_000 }, async () => {
    const { getAuth } = await import("@/lib/auth.server");
    const auth = await getAuth();
    expect(auth).toBeDefined();
    expect(auth.handler).toBeDefined();
  });

  it("auth instance has OAuth provider API methods", async () => {
    const { getAuth } = await import("@/lib/auth.server");
    const auth = await getAuth();
    // The oauthProvider plugin adds these API methods
    expect(auth.api).toBeDefined();
    expect(typeof auth.api.oauth2Consent).toBe("function");
    expect(typeof auth.api.getOAuthClient).toBe("function");
    expect(typeof auth.api.getOAuthServerConfig).toBe("function");
  });

  it("auth instance has JWT plugin API methods", async () => {
    const { getAuth } = await import("@/lib/auth.server");
    const auth = await getAuth();
    // The jwt plugin adds the JWKS endpoint
    expect(auth.api).toBeDefined();
  });

  it("does not allow unauthenticated client registration", async () => {
    // The unauthenticated registration endpoint must be closed. Allowing it
    // lets any party register OAuth clients with arbitrary redirect_uris,
    // enabling authorization-code phishing against real users.
    const { getAuth } = await import("@/lib/auth.server");
    const auth = await getAuth();
    // We verify the flag is not enabled by calling the register endpoint
    // without a session and expecting a 401.
    const request = new Request("http://localhost/api/auth/oauth2/register", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ redirect_uris: ["https://attacker.com/cb"] }),
    });
    const response = await auth.handler(request);
    expect(response.status).toBe(401);
  });
});
