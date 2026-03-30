import { queryOptions, useQuery } from "@tanstack/react-query";
import { createFileRoute, redirect } from "@tanstack/react-router";
import { createServerFn } from "@tanstack/react-start";
import { useState } from "react";
import { z } from "zod";
import AuthLayout from "@/components/(auth)/auth-layout";
import ErrorComponent from "@/components/common/error-component";
import NotFound from "@/components/common/not-found";
import { auth } from "@/lib/auth.server";
import {
  OAUTH_LOGIN_PAGE,
  OIDC_STANDARD_SCOPES,
  SCOPE_DESCRIPTIONS,
} from "@/lib/oauth-scopes";
import { captureException } from "@/lib/sentry";
import { authMiddleware } from "@/middlewares/auth";

type ScopeLevel = "read" | "write" | "admin";

const LEVEL_STYLES: Record<ScopeLevel, { bg: string; text: string }> = {
  read: { bg: "bg-blue-500/10", text: "text-blue-500" },
  write: { bg: "bg-amber-500/10", text: "text-amber-500" },
  admin: { bg: "bg-red-500/10", text: "text-red-500" },
};

const HIDDEN_SCOPES = new Set<string>(OIDC_STANDARD_SCOPES);

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

function extractHost(uri: string | undefined): string {
  if (!uri) {
    return "";
  }
  try {
    return new URL(uri).host;
  } catch {
    return uri;
  }
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

const consentSearchSchema = z.object({
  client_id: z.string().optional().catch(undefined),
  scope: z.string().optional().catch(undefined),
  redirect_uri: z.string().optional().catch(undefined),
  state: z.string().optional().catch(undefined),
  response_type: z.string().optional().catch(undefined),
  code_challenge: z.string().optional().catch(undefined),
  code_challenge_method: z.string().optional().catch(undefined),
});

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
      const client = await auth.api.getOAuthClient({
        query: { client_id: data.clientId },
      });
      if (!client) {
        return null;
      }
      return {
        name: String(client.name || "Unknown Application"),
        clientId: String(client.clientId || data.clientId),
        redirectUrls: Array.isArray(client.redirectURLs)
          ? (client.redirectURLs as string[])
          : [],
      } satisfies ClientInfo;
    } catch (err) {
      captureException(err, {
        tags: { feature: "oauth", action: "fetch_client" },
      });
      return null;
    }
  });

function clientInfoQueryOptions(clientId: string) {
  return queryOptions({
    queryKey: ["oauth-client", clientId],
    queryFn: () => fetchClientInfo({ data: { clientId } }),
    staleTime: 60_000,
    retry: false,
  });
}

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

export const Route = createFileRoute("/(auth)/oauth/consent")({
  validateSearch: consentSearchSchema,
  beforeLoad: ({ context, search }) => {
    if (!context.isAuthenticated) {
      const qs = buildSearchParams(
        search as Record<string, string | undefined>,
        OAUTH_QUERY_KEYS
      );
      throw redirect({
        to: OAUTH_LOGIN_PAGE,
        search: {
          redirect: `/oauth/consent${qs ? `?${qs}` : ""}`,
        },
      });
    }
  },
  loaderDeps: ({ search }) => ({ clientId: search.client_id }),
  loader: ({ context, deps }) => {
    if (deps.clientId) {
      return context.queryClient.ensureQueryData(
        clientInfoQueryOptions(deps.clientId)
      );
    }
    return null;
  },
  errorComponent: ErrorComponent,
  notFoundComponent: NotFound,
  component: OAuthConsentPage,
});

async function handleConsentSubmit(opts: {
  accept: boolean;
  scope: string | undefined;
  oauthQuery: string;
  redirectUri: string | undefined;
  state: string | undefined;
  setStatus: (s: "idle" | "authorizing" | "denying" | "error") => void;
  setError: (e: string | null) => void;
}) {
  opts.setStatus(opts.accept ? "authorizing" : "denying");
  opts.setError(null);

  try {
    const result = await submitConsent({
      data: {
        accept: opts.accept,
        scope: opts.scope,
        oauthQuery: opts.oauthQuery,
      },
    });

    if (result && typeof result === "object" && "url" in result) {
      window.location.href = (result as { url: string }).url;
      return;
    }

    if (!opts.accept && opts.redirectUri) {
      const url = new URL(opts.redirectUri);
      url.searchParams.set("error", "access_denied");
      url.searchParams.set(
        "error_description",
        "The user denied the authorization request"
      );
      if (opts.state) {
        url.searchParams.set("state", opts.state);
      }
      window.location.href = url.toString();
    }
  } catch (err) {
    captureException(err, {
      tags: { feature: "oauth", action: "consent_submit" },
    });
    opts.setStatus("error");
    opts.setError(
      err instanceof Error
        ? err.message
        : "Failed to process authorization request"
    );
  }
}

function OAuthConsentPage() {
  const search = Route.useSearch();
  const [status, setStatus] = useState<
    "idle" | "authorizing" | "denying" | "error"
  >("idle");
  const [error, setError] = useState<string | null>(null);

  const {
    data: clientInfo,
    isLoading: clientLoading,
    isError: clientError,
  } = useQuery({
    ...clientInfoQueryOptions(search.client_id ?? ""),
    enabled: !!search.client_id,
  });

  const { displayScopes, readScopes, writeScopes, adminScopes, unknownScopes } =
    parseScopes(search.scope);

  // Build the oauth_query string for the consent endpoint
  const oauthQuery = buildSearchParams(
    search as Record<string, string | undefined>,
    OAUTH_QUERY_KEYS
  );

  if (!(search.client_id && search.redirect_uri)) {
    return <ConsentMissingParams />;
  }

  if (clientLoading) {
    return <ConsentLoading />;
  }

  // -- Handlers ---------------------------------------------------------------

  function handleConsent(accept: boolean) {
    handleConsentSubmit({
      accept,
      scope: search.scope,
      oauthQuery,
      redirectUri: search.redirect_uri,
      state: search.state,
      setStatus,
      setError,
    });
  }

  // -- Render -----------------------------------------------------------------

  const clientName = clientInfo?.name ?? "Unknown Application";
  const redirectHost = extractHost(search.redirect_uri);

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

        {displayScopes.length > 0 ? (
          <PermissionsList
            adminScopes={adminScopes}
            readScopes={readScopes}
            unknownScopes={unknownScopes}
            writeScopes={writeScopes}
          />
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
                <span className="size-4 animate-spin rounded-full border-2 border-primary-foreground/30 border-t-primary-foreground" />
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

function PermissionsList({
  readScopes,
  writeScopes,
  adminScopes,
  unknownScopes,
}: {
  readScopes: string[];
  writeScopes: string[];
  adminScopes: string[];
  unknownScopes: string[];
}) {
  return (
    <div className="rounded-lg border border-border bg-muted/30 p-4">
      <p className="mb-3 font-medium text-foreground text-sm">
        Permissions requested
      </p>
      <div className="flex flex-col gap-2.5">
        {readScopes.length > 0 ? (
          <ScopeGroup level="read" scopes={readScopes} />
        ) : null}
        {writeScopes.length > 0 ? (
          <ScopeGroup level="write" scopes={writeScopes} />
        ) : null}
        {adminScopes.length > 0 ? (
          <ScopeGroup level="admin" scopes={adminScopes} />
        ) : null}
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
                  <span className="text-muted-foreground text-sm">{scope}</span>
                </div>
              ))}
            </div>
          </div>
        ) : null}
      </div>
      <div className="mt-3 border-border border-t pt-3">
        <p className="text-muted-foreground text-xs">
          This app will <span className="font-medium text-foreground">not</span>{" "}
          be able to manage API keys, change account settings, or access billing
          information.
        </p>
      </div>
    </div>
  );
}

function ConsentMissingParams() {
  return (
    <AuthLayout
      description="The authorization request is missing required parameters."
      title="Invalid Request"
    >
      <p className="text-center text-muted-foreground text-sm">
        This page should be accessed through an OAuth authorization flow. Please
        try again from the application you were using.
      </p>
    </AuthLayout>
  );
}

function ConsentLoading() {
  return (
    <AuthLayout
      description="Loading application details..."
      title="Authorize Application"
    >
      <div className="flex items-center justify-center py-8">
        <div className="size-6 animate-spin rounded-full border-2 border-muted-foreground/30 border-t-primary" />
      </div>
    </AuthLayout>
  );
}

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
