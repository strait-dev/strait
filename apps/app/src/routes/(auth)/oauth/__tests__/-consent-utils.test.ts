import { describe, expect, it } from "vitest";
import { resolveRedirectHost } from "../-consent-utils";

// extractHost is an internal helper — tested indirectly through resolveRedirectHost
// to cover its own edge cases without exporting it.

describe("resolveRedirectHost", () => {
  it("returns host from server-authoritative clientInfo when available", () => {
    const clientInfo = {
      name: "My App",
      clientId: "abc",
      redirectUrls: ["https://myapp.example.com/callback"],
    };
    const result = resolveRedirectHost(clientInfo, "https://attacker.com/cb");
    expect(result).toBe("myapp.example.com");
  });

  it("returns empty string when clientInfo is null", () => {
    const result = resolveRedirectHost(null, "https://attacker.com/cb");
    expect(result).toBe("");
  });

  it("returns empty string when clientInfo is undefined", () => {
    const result = resolveRedirectHost(undefined, "https://attacker.com/cb");
    expect(result).toBe("");
  });

  it("returns empty string when clientInfo has no redirect URLs", () => {
    const clientInfo = { name: "My App", clientId: "abc", redirectUrls: [] };
    const result = resolveRedirectHost(clientInfo, "https://attacker.com/cb");
    expect(result).toBe("");
  });

  it("returns host from first registered URL, ignoring additional URLs", () => {
    const clientInfo = {
      name: "My App",
      clientId: "abc",
      redirectUrls: [
        "https://first.example.com/cb",
        "https://second.example.com/cb",
      ],
    };
    const result = resolveRedirectHost(clientInfo, "https://attacker.com/cb");
    expect(result).toBe("first.example.com");
  });

  it("returns the raw string for a malformed registered redirect URL, never attacker.com", () => {
    const clientInfo = {
      name: "My App",
      clientId: "abc",
      redirectUrls: ["not-a-url"],
    };
    const result = resolveRedirectHost(clientInfo, "https://attacker.com/cb");
    // extractHost falls back to the raw string when URL() throws.
    expect(result).toBe("not-a-url");
  });

  // extractHost edge cases — exercised through resolveRedirectHost
  it("includes port in host when the registered URL specifies one", () => {
    const clientInfo = {
      name: "My App",
      clientId: "abc",
      redirectUrls: ["https://app.example.com:8443/callback"],
    };
    const result = resolveRedirectHost(clientInfo, "https://attacker.com/cb");
    expect(result).toBe("app.example.com:8443");
  });

  it("returns empty string when the registered URL is an empty string", () => {
    const clientInfo = {
      name: "My App",
      clientId: "abc",
      redirectUrls: [""],
    };
    const result = resolveRedirectHost(clientInfo, "https://attacker.com/cb");
    expect(result).toBe("");
  });
});
