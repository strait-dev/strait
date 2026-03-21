import { describe, expect, it } from "vitest";
import { getOrgIdFromSession } from "./session";

describe("getOrgIdFromSession", () => {
  it("returns the org ID when present and a string", () => {
    const session = { activeOrganizationId: "org_123" };
    expect(getOrgIdFromSession(session)).toBe("org_123");
  });

  it("returns null when activeOrganizationId is missing", () => {
    const session = {};
    expect(getOrgIdFromSession(session)).toBeNull();
  });

  it("returns null when activeOrganizationId is undefined", () => {
    const session = { activeOrganizationId: undefined };
    expect(getOrgIdFromSession(session)).toBeNull();
  });

  it("returns null when activeOrganizationId is null", () => {
    const session = { activeOrganizationId: null };
    expect(getOrgIdFromSession(session)).toBeNull();
  });

  it("returns null when activeOrganizationId is a number", () => {
    const session = { activeOrganizationId: 42 };
    expect(getOrgIdFromSession(session)).toBeNull();
  });

  it("returns null when activeOrganizationId is an empty string", () => {
    const session = { activeOrganizationId: "" };
    expect(getOrgIdFromSession(session)).toBeNull();
  });

  it("returns the ID for non-empty strings with special characters", () => {
    const session = { activeOrganizationId: "org-abc_123-def" };
    expect(getOrgIdFromSession(session)).toBe("org-abc_123-def");
  });
});
