import { describe, expect, it } from "vitest";
import {
  ALL_OAUTH_SCOPES,
  OIDC_STANDARD_SCOPES,
  SCOPE_DESCRIPTIONS,
} from "@/lib/oauth-scopes";

/**
 * These are the scopes defined in the Go backend at
 * apps/strait/internal/domain/scopes.go.
 *
 * The OAuth provider must expose a subset of these (excluding admin-only
 * scopes like *, api-keys:manage, rbac:manage).
 */
const GO_BACKEND_SCOPES = [
  "*",
  "jobs:read",
  "jobs:write",
  "jobs:trigger",
  "runs:read",
  "runs:write",
  "workflows:read",
  "workflows:write",
  "workflows:trigger",
  "secrets:read",
  "secrets:write",
  "api-keys:manage",
  "rbac:manage",
  "stats:read",
  "projects:read",
  "projects:write",
  "projects:manage",
];

const ADMIN_ONLY_SCOPES = ["*", "api-keys:manage", "rbac:manage"];

// Use the shared module as the source of truth.
const OAUTH_PROVIDER_SCOPES = [...ALL_OAUTH_SCOPES];

describe("OAuth scope configuration", () => {
  it("includes all non-admin Go backend scopes", () => {
    const nonAdminGoScopes = GO_BACKEND_SCOPES.filter(
      (s) => !ADMIN_ONLY_SCOPES.includes(s)
    );
    for (const scope of nonAdminGoScopes) {
      expect(OAUTH_PROVIDER_SCOPES, `Missing scope: ${scope}`).toContain(scope);
    }
  });

  it("excludes admin-only scopes from OAuth provider", () => {
    for (const scope of ADMIN_ONLY_SCOPES) {
      expect(
        OAUTH_PROVIDER_SCOPES,
        `Admin scope should not be exposed: ${scope}`
      ).not.toContain(scope);
    }
  });

  it("includes standard OIDC scopes", () => {
    for (const scope of OIDC_STANDARD_SCOPES) {
      expect(OAUTH_PROVIDER_SCOPES).toContain(scope);
    }
  });

  it("has no duplicate scopes", () => {
    const unique = new Set(OAUTH_PROVIDER_SCOPES);
    expect(unique.size).toBe(OAUTH_PROVIDER_SCOPES.length);
  });
});

describe("consent page scope descriptions", () => {
  const HIDDEN_SCOPES = new Set<string>(OIDC_STANDARD_SCOPES);

  it("has a description for every non-OIDC OAuth scope", () => {
    const straitScopes = OAUTH_PROVIDER_SCOPES.filter(
      (s) => !HIDDEN_SCOPES.has(s)
    );
    for (const scope of straitScopes) {
      expect(
        SCOPE_DESCRIPTIONS,
        `Missing description for scope: ${scope}`
      ).toHaveProperty(scope);
    }
  });

  it("does not have descriptions for OIDC standard scopes", () => {
    for (const scope of OIDC_STANDARD_SCOPES) {
      expect(SCOPE_DESCRIPTIONS).not.toHaveProperty(scope);
    }
  });

  it("every description has a non-empty label", () => {
    for (const [scope, info] of Object.entries(SCOPE_DESCRIPTIONS)) {
      expect(info.label, `Empty label for ${scope}`).toBeTruthy();
    }
  });

  it("every description has a non-empty description", () => {
    for (const [scope, info] of Object.entries(SCOPE_DESCRIPTIONS)) {
      expect(info.description, `Empty description for ${scope}`).toBeTruthy();
    }
  });

  it("every description has a valid level", () => {
    const validLevels = new Set(["read", "write", "admin"]);
    for (const [scope, info] of Object.entries(SCOPE_DESCRIPTIONS)) {
      expect(
        validLevels.has(info.level),
        `Invalid level "${info.level}" for ${scope}`
      ).toBe(true);
    }
  });

  it("read scopes have read level", () => {
    const readScopes = Object.entries(SCOPE_DESCRIPTIONS).filter(([scope]) =>
      scope.endsWith(":read")
    );
    for (const [scope, info] of readScopes) {
      expect(info.level, `${scope} should be read level`).toBe("read");
    }
  });

  it("secrets:write is admin level (elevated permission)", () => {
    expect(SCOPE_DESCRIPTIONS["secrets:write"]?.level).toBe("admin");
  });

  it("projects:manage is admin level (elevated permission)", () => {
    expect(SCOPE_DESCRIPTIONS["projects:manage"]?.level).toBe("admin");
  });
});
