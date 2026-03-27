import { describe, expect, it } from "vitest";

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

/**
 * Scopes configured in the oauthProvider plugin (auth.server.ts).
 * Kept in sync manually — this test catches drift.
 */
const OAUTH_PROVIDER_SCOPES = [
  "openid",
  "profile",
  "email",
  "offline_access",
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
  "stats:read",
  "projects:read",
  "projects:write",
  "projects:manage",
];

const OIDC_STANDARD_SCOPES = ["openid", "profile", "email", "offline_access"];

describe("OAuth scope configuration", () => {
  it("includes all non-admin Go backend scopes", () => {
    const nonAdminGoScopes = GO_BACKEND_SCOPES.filter(
      (s) => !ADMIN_ONLY_SCOPES.includes(s)
    );
    for (const scope of nonAdminGoScopes) {
      expect(
        OAUTH_PROVIDER_SCOPES,
        `Missing scope: ${scope}`
      ).toContain(scope);
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
  /**
   * Scope descriptions from the consent page component.
   * This ensures every Strait API scope exposed via OAuth has a
   * human-readable description for the consent screen.
   */
  const SCOPE_DESCRIPTIONS: Record<
    string,
    { label: string; description: string; level: "read" | "write" | "admin" }
  > = {
    "jobs:read": {
      label: "View jobs",
      description: "View your jobs and their configurations",
      level: "read",
    },
    "jobs:write": {
      label: "Modify jobs",
      description: "Create, update, and delete jobs",
      level: "write",
    },
    "jobs:trigger": {
      label: "Trigger jobs",
      description: "Trigger job executions manually",
      level: "write",
    },
    "runs:read": {
      label: "View runs",
      description: "View job run history and logs",
      level: "read",
    },
    "runs:write": {
      label: "Modify runs",
      description: "Cancel or retry runs",
      level: "write",
    },
    "workflows:read": {
      label: "View workflows",
      description: "View workflows and their definitions",
      level: "read",
    },
    "workflows:write": {
      label: "Modify workflows",
      description: "Create, update, and delete workflows",
      level: "write",
    },
    "workflows:trigger": {
      label: "Trigger workflows",
      description: "Trigger workflow executions",
      level: "write",
    },
    "secrets:read": {
      label: "View secrets",
      description: "View secret names (values are never exposed)",
      level: "read",
    },
    "secrets:write": {
      label: "Modify secrets",
      description: "Create and update secrets",
      level: "admin",
    },
    "stats:read": {
      label: "View statistics",
      description: "View usage and performance statistics",
      level: "read",
    },
    "projects:read": {
      label: "View projects",
      description: "View project details and settings",
      level: "read",
    },
    "projects:write": {
      label: "Modify projects",
      description: "Update project settings",
      level: "write",
    },
    "projects:manage": {
      label: "Manage projects",
      description: "Full project management including deletion",
      level: "admin",
    },
  };

  const HIDDEN_SCOPES = new Set([
    "openid",
    "profile",
    "email",
    "offline_access",
  ]);

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
    const readScopes = Object.entries(SCOPE_DESCRIPTIONS).filter(
      ([scope]) => scope.endsWith(":read")
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
