import { describe, expect, it, vi } from "vitest";

vi.mock("@/lib/auth.server", () => ({
  getAuth: async () => ({
    api: {
      getOAuthServerConfig: async () => ({ issuer: "https://api.strait.dev" }),
      getOpenIdConfig: async () => ({
        issuer: "https://api.strait.dev",
        jwks_uri: "https://api.strait.dev/api/auth/jwks",
      }),
    },
  }),
}));

import {
  handleWellKnownOAuthRequest,
  isWellKnownOAuthRequest,
} from "../well-known-oauth.server";

describe("well-known OAuth metadata", () => {
  it("detects well-known metadata requests", () => {
    expect(
      isWellKnownOAuthRequest(
        new Request("https://app.strait.dev/.well-known/openid-configuration")
      )
    ).toBe(true);
    expect(
      isWellKnownOAuthRequest(new Request("https://app.strait.dev/app"))
    ).toBe(false);
  });

  it("serves OAuth authorization server metadata", async () => {
    const response = await handleWellKnownOAuthRequest(
      new Request(
        "https://app.strait.dev/.well-known/oauth-authorization-server"
      )
    );

    expect(response?.status).toBe(200);
    await expect(response?.json()).resolves.toEqual({
      issuer: "https://api.strait.dev",
    });
  });

  it("allows CORS preflight requests", async () => {
    const response = await handleWellKnownOAuthRequest(
      new Request("https://app.strait.dev/.well-known/openid-configuration", {
        method: "OPTIONS",
      })
    );

    expect(response?.status).toBe(204);
    expect(response?.headers.get("access-control-allow-origin")).toBe("*");
  });
});
