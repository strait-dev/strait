import { createFileRoute, redirect } from "@tanstack/react-router";
import { createServerFn } from "@tanstack/react-start";
import { useState } from "react";
import { z } from "zod";
import AuthLayout from "@/components/(auth)/auth-layout";
import ErrorComponent from "@/components/common/error-component";
import NotFound from "@/components/common/not-found";
import { auth } from "@/lib/auth.server";
import { authMiddleware } from "@/middlewares/auth";

// -- Scope metadata -----------------------------------------------------------

type ScopeLevel = "read" | "write" | "admin";

type ScopeInfo = {
  label: string;
  description: string;
  level: ScopeLevel;
};

const SCOPE_DESCRIPTIONS: Record<string, ScopeInfo> = {
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

const LEVEL_STYLES: Record<ScopeLevel, { bg: string; text: string }> = {
  read: { bg: "bg-blue-500/10", text: "text-blue-500" },
  write: { bg: "bg-amber-500/10", text: "text-amber-500" },
  admin: { bg: "bg-red-500/10", text: "text-red-500" },
};

// Scopes that are standard OIDC and not displayed in the permissions list
const HIDDEN_SCOPES = new Set([
  "openid",
  "profile",
  "email",
  "offline_access",
]);

// -- Helpers ------------------------------------------------------------------

function buildSearchParams(
  search: Record<string, string | undefined>,
  keys: string[]
): string {
  const params = new URLSearchParams();
  for (const key of keys) {
    const value = search[key];
    if (value) {
      params.set(key, value);
    }
  }
  return params.toString();
}

function parseScopes(scopeString: string | undefined) {
  const requested = (scopeString ?? "").split(" ").filter((s) => s.length > 0);
  const displayScopes = requested.filter((s) => !HIDDEN_SCOPES.has(s));
  return {
    displayScopes,
    readScopes: displayScopes.filter(
      (s) => SCOPE_DESCRIPTIONS[s]?.level === "read"
    ),
    writeScopes: displayScopes.filter(
      (s) => SCOPE_DESCRIPTIONS[s]?.level === "write"
    ),
    adminScopes: displayScopes.filter(
      (s) => SCOPE_DESCRIPTIONS[s]?.level === "admin"
    ),
    unknownScopes: displayScopes.filter((s) => !SCOPE_DESCRIPTIONS[s]),
  };
}

const OAUTH_QUERY_KEYS = [
  "response_type",
  "client_id",
  "redirect_uri",
  "scope",
  "state",
  "code_challenge",
  "code_challenge_method",
];

// -- Search schema ------------------------------------------------------------

const consentSearchSchema = z.object({
  client_id: z.string().optional().catch(undefined),
  scope: z.string().optional().catch(undefined),
  redirect_uri: z.string().optional().catch(undefined),
  state: z.string().optional().catch(undefined),
  response_type: z.string().optional().catch(undefined),
  code_challenge: z.string().optional().catch(undefined),
  code_challenge_method: z.string().optional().catch(undefined),
});

// -- Server functions ---------------------------------------------------------

type ClientInfo = {
  name: string;
  clientId: string;
  redirectUrls: string[];
};

const fetchClientInfo = createServerFn({ method: "GET" })
  .inputValidator(z.object({ clientId: z.string().min(1) }))
  .middleware([authMiddleware])
  .handler(async ({ data }) => {
    try {
      const client = await (auth.api as any).getOAuthClient({
        body: { client_id: data.clientId },
      });
      if (!client) {
        return null;
      }
      return {
        name: (client as any).name ?? "Unknown Application",
        clientId: (client as any).clientId ?? data.clientId,
        redirectUrls: (client as any).redirectURLs ?? [],
      } satisfies ClientInfo;
    } catch {
      return null;
    }
  });

const submitConsent = createServerFn({ method: "POST" })
  .inputValidator(
    z.object({
      accept: z.boolean(),
      scope: z.string().optional(),
      oauthQuery: z.string().optional(),
    })
  )
  .middleware([authMiddleware])
  .handler(async ({ data }) => {
    const result = await auth.api.oauth2Consent({
      body: {
        accept: data.accept,
        scope: data.scope,
        oauth_query: data.oauthQuery,
      },
    });
    return result;
  });

// -- Route --------------------------------------------------------------------

export const Route = createFileRoute("/(auth)/oauth/consent")({
  validateSearch: consentSearchSchema,
  beforeLoad: ({ context, search }) => {
    if (!context.isAuthenticated) {
      const qs = buildSearchParams(search as Record<string, string | undefined>, OAUTH_QUERY_KEYS);
      throw redirect({
        to: "/login",
        search: {
          redirect: `/oauth/consent${qs ? `?${qs}` : ""}`,
        },
      });
    }
  },
  errorComponent: ErrorComponent,
  notFoundComponent: NotFound,
  component: OAuthConsentPage,
});

// -- Page component -----------------------------------------------------------

function OAuthConsentPage() {
  const search = Route.useSearch();
  const [status, setStatus] = useState<
    "idle" | "authorizing" | "denying" | "error"
  >("idle");
  const [error, setError] = useState<string | null>(null);
  const [clientInfo, setClientInfo] = useState<ClientInfo | null>(null);
  const [clientLoading, setClientLoading] = useState(true);
  const [clientError, setClientError] = useState(false);

  // Fetch client info on mount
  useState(() => {
    if (search.client_id) {
      fetchClientInfo({ data: { clientId: search.client_id } })
        .then((info) => {
          setClientInfo(info);
          setClientLoading(false);
        })
        .catch(() => {
          setClientError(true);
          setClientLoading(false);
        });
    } else {
      setClientLoading(false);
      setClientError(true);
    }
  });

  const { displayScopes, readScopes, writeScopes, adminScopes, unknownScopes } =
    parseScopes(search.scope);

  // Build the oauth_query string for the consent endpoint
  const oauthQuery = buildSearchParams(
    search as Record<string, string | undefined>,
    OAUTH_QUERY_KEYS
  );

  // -- Missing params ---------------------------------------------------------

  if (!(search.client_id && search.redirect_uri)) {
    return (
      <AuthLayout
        description="The authorization request is missing required parameters."
        title="Invalid Request"
      >
        <p className="text-center text-muted-foreground text-sm">
          This page should be accessed through an OAuth authorization flow.
          Please try again from the application you were using.
        </p>
      </AuthLayout>
    );
  }

  // -- Loading state ----------------------------------------------------------

  if (clientLoading) {
    return (
      <AuthLayout
        description="Loading application details..."
        title="Authorize Application"
      >
        <div className="flex items-center justify-center py-8">
          <div className="h-6 w-6 animate-spin rounded-full border-2 border-muted-foreground/30 border-t-primary" />
        </div>
      </AuthLayout>
    );
  }

  // -- Handlers ---------------------------------------------------------------

  async function handleConsent(accept: boolean) {
    setStatus(accept ? "authorizing" : "denying");
    setError(null);

    try {
      const result = await submitConsent({
        data: {
          accept,
          scope: search.scope,
          oauthQuery,
        },
      });

      // The consent endpoint returns a redirect URL
      if (result && typeof result === "object" && "url" in result) {
        window.location.href = (result as { url: string }).url;
        return;
      }

      // If denied, redirect back with error
      if (!accept && search.redirect_uri) {
        const url = new URL(search.redirect_uri);
        url.searchParams.set("error", "access_denied");
        url.searchParams.set(
          "error_description",
          "The user denied the authorization request"
        );
        if (search.state) {
          url.searchParams.set("state", search.state);
        }
        window.location.href = url.toString();
        return;
      }
    } catch (err) {
      setStatus("error");
      setError(
        err instanceof Error
          ? err.message
          : "Failed to process authorization request"
      );
    }
  }

  // -- Render -----------------------------------------------------------------

  const clientName = clientInfo?.name ?? "Unknown Application";
  const redirectHost = (() => {
    try {
      return new URL(search.redirect_uri).host;
    } catch {
      return search.redirect_uri;
    }
  })();

  return (
    <AuthLayout
      description={`"${clientName}" wants access to your Strait account`}
      title="Authorize Application"
    >
      <div className="flex flex-col gap-4">
        {/* Client warning for unrecognized clients */}
        {clientError || !clientInfo ? (
          <div
            className="rounded-md bg-amber-500/10 p-3 text-amber-600 text-sm dark:text-amber-400"
            role="alert"
          >
            Unable to verify this application. Proceed with caution.
          </div>
        ) : null}

        {/* Permissions list */}
        {displayScopes.length > 0 ? (
          <div className="rounded-lg border border-border bg-muted/30 p-4">
            <p className="mb-3 font-medium text-foreground text-sm">
              Permissions requested
            </p>

            <div className="flex flex-col gap-2.5">
              {/* Read permissions */}
              {readScopes.length > 0 ? (
                <ScopeGroup level="read" scopes={readScopes} />
              ) : null}

              {/* Write permissions */}
              {writeScopes.length > 0 ? (
                <ScopeGroup level="write" scopes={writeScopes} />
              ) : null}

              {/* Admin permissions */}
              {adminScopes.length > 0 ? (
                <ScopeGroup level="admin" scopes={adminScopes} />
              ) : null}

              {/* Unknown scopes */}
              {unknownScopes.length > 0 ? (
                <div className="mt-1">
                  <div className="flex items-center gap-1.5">
                    <span className="inline-flex rounded-full bg-muted px-2 py-0.5 font-medium text-[10px] text-muted-foreground uppercase tracking-wider">
                      other
                    </span>
                  </div>
                  <div className="mt-1.5 flex flex-col gap-1 pl-1">
                    {unknownScopes.map((scope) => (
                      <div className="flex items-start gap-2" key={scope}>
                        <span className="mt-1.5 h-1 w-1 shrink-0 rounded-full bg-muted-foreground/50" />
                        <span className="text-muted-foreground text-sm">
                          {scope}
                        </span>
                      </div>
                    ))}
                  </div>
                </div>
              ) : null}
            </div>

            {/* Safety notice */}
            <div className="mt-3 border-border border-t pt-3">
              <p className="text-muted-foreground text-xs">
                This app will{" "}
                <span className="font-medium text-foreground">not</span> be able
                to manage API keys, change account settings, or access billing
                information.
              </p>
            </div>
          </div>
        ) : null}

        {/* Redirect URI display */}
        <div className="flex items-center gap-2 rounded-md bg-muted/50 px-3 py-2">
          <svg
            className="h-3.5 w-3.5 shrink-0 text-muted-foreground"
            fill="none"
            stroke="currentColor"
            strokeWidth={2}
            viewBox="0 0 24 24"
          >
            <path
              d="M13.828 10.172a4 4 0 00-5.656 0l-4 4a4 4 0 105.656 5.656l1.102-1.101"
              strokeLinecap="round"
              strokeLinejoin="round"
            />
            <path
              d="M10.172 13.828a4 4 0 005.656 0l4-4a4 4 0 00-5.656-5.656l-1.102 1.101"
              strokeLinecap="round"
              strokeLinejoin="round"
            />
          </svg>
          <span className="truncate text-muted-foreground text-xs">
            Redirects to{" "}
            <span className="font-medium text-foreground">{redirectHost}</span>
          </span>
        </div>

        {/* Error display */}
        {error ? (
          <div
            className="rounded-md bg-destructive/10 p-3 text-destructive text-sm"
            role="alert"
          >
            {error}
          </div>
        ) : null}

        {/* Action buttons */}
        <div className="flex w-full gap-3">
          <button
            className="flex-1 rounded-custom border border-border bg-background px-4 py-2.5 font-medium text-foreground text-sm transition-colors hover:bg-muted disabled:opacity-50"
            disabled={status === "authorizing" || status === "denying"}
            onClick={() => handleConsent(false)}
            type="button"
          >
            {status === "denying" ? "Denying..." : "Deny"}
          </button>
          <button
            className="flex-1 rounded-custom bg-primary px-4 py-2.5 font-medium text-primary-foreground text-sm transition-colors hover:bg-primary/90 disabled:opacity-50"
            disabled={status === "authorizing" || status === "denying"}
            onClick={() => handleConsent(true)}
            type="button"
          >
            {status === "authorizing" ? (
              <span className="flex items-center justify-center gap-2">
                <span className="h-4 w-4 animate-spin rounded-full border-2 border-primary-foreground/30 border-t-primary-foreground" />
                Authorizing...
              </span>
            ) : (
              "Authorize"
            )}
          </button>
        </div>

        {/* Footer notice */}
        <p className="text-center text-muted-foreground text-xs">
          Authorizing gives this app access to your data based on the
          permissions above. You can revoke access at any time in Settings.
        </p>
      </div>
    </AuthLayout>
  );
}

// -- Scope group component ----------------------------------------------------

function ScopeGroup({
  level,
  scopes,
}: {
  level: ScopeLevel;
  scopes: string[];
}) {
  const styles = LEVEL_STYLES[level];
  const labels: Record<ScopeLevel, string> = {
    read: "Read",
    write: "Write",
    admin: "Admin",
  };

  return (
    <div>
      <div className="flex items-center gap-1.5">
        <span
          className={`inline-flex rounded-full px-2 py-0.5 font-medium text-[10px] uppercase tracking-wider ${styles.bg} ${styles.text}`}
        >
          {labels[level]}
        </span>
      </div>
      <div className="mt-1.5 flex flex-col gap-1 pl-1">
        {scopes.map((scope) => {
          const info = SCOPE_DESCRIPTIONS[scope];
          if (!info) {
            return null;
          }
          return (
            <div className="flex items-start gap-2" key={scope}>
              <span
                className={`mt-1.5 h-1 w-1 shrink-0 rounded-full ${styles.text.replace("text-", "bg-")}`}
              />
              <div className="flex flex-col">
                <span className="text-foreground text-sm">{info.label}</span>
                <span className="text-muted-foreground text-xs">
                  {info.description}
                </span>
              </div>
            </div>
          );
        })}
      </div>
    </div>
  );
}
