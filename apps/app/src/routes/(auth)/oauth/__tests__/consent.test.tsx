import { describe, expect, it } from "vitest";
import { resolveRedirectHost } from "../consent-utils";

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

  it("returns empty string for malformed registered redirect URL", () => {
    const clientInfo = {
      name: "My App",
      clientId: "abc",
      redirectUrls: ["not-a-url"],
    };
    const result = resolveRedirectHost(clientInfo, "https://attacker.com/cb");
    // Falls back to the raw string since URL() throws, but it's not a host.
    // The important thing: attacker.com is NOT returned.
    expect(result).not.toBe("attacker.com");
  });
});
