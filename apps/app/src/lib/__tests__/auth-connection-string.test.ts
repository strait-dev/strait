import { afterEach, describe, expect, it, vi } from "vitest";

vi.mock("pg", () => ({
  Pool: class MockPool {
    config: { connectionString?: string };

    constructor(config: { connectionString?: string }) {
      this.config = config;
    }

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

vi.mock("@/lib/stripe.server", () => ({
  findOrCreateCustomerForOrg: vi.fn(),
}));

describe("getAuthConnectionString", () => {
  afterEach(() => {
    vi.unstubAllEnvs();
    vi.resetModules();
  });

  it("returns AUTH_DATABASE_URL", async () => {
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
  }, 15_000);

  it("returns an empty string when AUTH_DATABASE_URL is unset", async () => {
    vi.stubEnv("AUTH_DATABASE_URL", "");
    vi.stubEnv("BETTER_AUTH_URL", "http://localhost:3000");
    vi.stubEnv("BETTER_AUTH_SECRET", "test-secret-at-least-32-chars-long");

    const { getAuthConnectionString } = await import("@/lib/auth.server");
    expect(getAuthConnectionString()).toBe("");
  }, 15_000);

  it("initializes the auth pool with AUTH_DATABASE_URL", async () => {
    vi.stubEnv(
      "AUTH_DATABASE_URL",
      "postgres://selfhost:secret@localhost:5432/strait"
    );
    vi.stubEnv("BETTER_AUTH_URL", "http://localhost:3000");
    vi.stubEnv("BETTER_AUTH_SECRET", "test-secret-at-least-32-chars-long");

    const { getAuthPool } = await import("@/lib/auth.server");
    expect(getAuthPool()).toMatchObject({
      config: {
        connectionString: "postgres://selfhost:secret@localhost:5432/strait",
      },
    });
  }, 15_000);
});
