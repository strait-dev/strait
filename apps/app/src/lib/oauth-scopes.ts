/**
 * Single source of truth for OAuth configuration across the app.
 *
 * Scopes must stay aligned with:
 * - OAuth server: SUPPORTED_SCOPES in src/auth.ts
 * - Go API: domain.ValidScopes in internal/domain/scopes.go
 * - oauthProvider config in src/lib/auth.server.ts
 */

export const OIDC_KEY_ID = "oidc-rsa-1";
export const OIDC_ALGORITHM = "RS256";

export const OAUTH_LOGIN_PAGE = "/login";
export const OAUTH_CONSENT_PAGE = "/oauth/consent";

export const DEFAULT_REGISTRATION_SCOPES = [
  "openid",
  "profile",
  "jobs:read",
  "runs:read",
  "stats:read",
] as const;

export const OAUTH_CORS_HEADERS = {
  "Access-Control-Allow-Origin": "*",
  "Access-Control-Allow-Methods": "GET, OPTIONS",
  "Cache-Control": "public, max-age=3600",
} as const;

export const OAUTH_API_CORS_HEADERS = {
  "Access-Control-Allow-Origin": "*",
  "Access-Control-Allow-Methods": "GET, POST, OPTIONS",
  "Access-Control-Allow-Headers": "Content-Type, Authorization",
  "Access-Control-Max-Age": "86400",
} as const;

export const STRAIT_API_SCOPES = [
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
  "webhooks:read",
  "webhooks:write",
  "projects:read",
  "projects:write",
  "projects:manage",
] as const;

export const OIDC_STANDARD_SCOPES = [
  "openid",
  "profile",
  "email",
  "offline_access",
] as const;

export const ALL_OAUTH_SCOPES = [
  ...OIDC_STANDARD_SCOPES,
  ...STRAIT_API_SCOPES,
] as const;

const VALID_OAUTH_SCOPES = new Set<string>(ALL_OAUTH_SCOPES);

const SCOPE_SPLIT_RE = /[,\s]+/;

export function parseAndFilterScopes(scopes: unknown): string[] {
  let raw: string[] = [];
  if (typeof scopes === "string") {
    raw = scopes.split(SCOPE_SPLIT_RE).filter(Boolean);
  } else if (Array.isArray(scopes)) {
    raw = scopes.map(String).filter(Boolean);
  }
  return raw.filter((s) => VALID_OAUTH_SCOPES.has(s));
}

export const SCOPE_DESCRIPTIONS: Record<
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
  "webhooks:read": {
    label: "View webhooks",
    description: "View webhook subscriptions and delivery history",
    level: "read",
  },
  "webhooks:write": {
    label: "Modify webhooks",
    description: "Create, update, and delete webhook subscriptions",
    level: "write",
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
