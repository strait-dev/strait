import { afterEach, describe, expect, it, vi } from "vitest";

/**
 * Covers the Hyperdrive → AUTH_DATABASE_URL fallback in auth.server.ts.
 *
 * In Cloudflare Workers the Hyperdrive binding is populated via
 * `cloudflare:workers` and takes precedence. In Node (self-host Docker
 * build) the binding does not exist and the code falls through to
 * `process.env.AUTH_DATABASE_URL`. STR-398 requires explicit coverage
 * so the fallback path cannot silently regress.
 */

// `cloudflare:workers` itself is resolved by the virtual-module shim
// registered in apps/app/vitest.config.ts — tests get an empty `env: {}`
// by default. Individual test cases override that with `vi.doMock` when
// they need a populated Hyperdrive binding.

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
  Client: class MockClient {
    connect = vi.fn().mockResolvedValue(undefined);
    query = vi.fn().mockResolvedValue({ rows: [], rowCount: 0 });
    end = vi.fn().mockResolvedValue(undefined);
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

describe("getAuthConnectionString — Hyperdrive → AUTH_DATABASE_URL fallback", () => {
  afterEach(() => {
    vi.unstubAllEnvs();
    vi.resetModules();
  });

  it("returns AUTH_DATABASE_URL when no Hyperdrive binding is present", async () => {
    vi.stubEnv(
      "AUTH_DATABASE_URL",
      "postgres://selfhost:secret@localhost:5432/strait"
    );
    vi.stubEnv("BETTER_AUTH_URL", "http://localhost:3000");
    vi.stubEnv("BETTER_AUTH_SECRET", "test-secret-at-least-32-chars-long");

    const { getAuthConnectionString } = await import("@/lib/auth.server");
    expect(getAuthConnectionString()).toBe(
      "postgres://selfhost:secret@localhost:5432/strait"
    );
  });

  it("returns an empty string when neither Hyperdrive nor AUTH_DATABASE_URL is set", async () => {
    vi.stubEnv("AUTH_DATABASE_URL", "");
    vi.stubEnv("BETTER_AUTH_URL", "http://localhost:3000");
    vi.stubEnv("BETTER_AUTH_SECRET", "test-secret-at-least-32-chars-long");

    const { getAuthConnectionString } = await import("@/lib/auth.server");
    expect(getAuthConnectionString()).toBe("");
  });

  it("prefers a Hyperdrive binding over AUTH_DATABASE_URL when both are present", async () => {
    vi.resetModules();
    vi.doMock("cloudflare:workers", () => ({
      env: {
        HYPERDRIVE: {
          connectionString: "postgres://hyperdrive:proxy@cf-edge/strait",
        },
      },
    }));

    vi.stubEnv(
      "AUTH_DATABASE_URL",
      "postgres://selfhost:secret@localhost:5432/strait"
    );
    vi.stubEnv("BETTER_AUTH_URL", "http://localhost:3000");
    vi.stubEnv("BETTER_AUTH_SECRET", "test-secret-at-least-32-chars-long");

    const { getAuthConnectionString } = await import("@/lib/auth.server");
    expect(getAuthConnectionString()).toBe(
      "postgres://hyperdrive:proxy@cf-edge/strait"
    );

    vi.doUnmock("cloudflare:workers");
  });
});
