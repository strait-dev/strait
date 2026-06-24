import { HugeiconsIcon } from "@hugeicons/react";
import { Alert, AlertDescription } from "@strait/ui/components/alert";
import type { BadgeProps } from "@strait/ui/components/badge";
import { Badge } from "@strait/ui/components/badge";
import { Button } from "@strait/ui/components/button";
import { Frame, FramePanel } from "@strait/ui/components/frame";
import { Spinner } from "@strait/ui/components/spinner";
import { queryOptions, useQuery } from "@tanstack/react-query";
import { createFileRoute, redirect } from "@tanstack/react-router";
import { createServerFn } from "@tanstack/react-start";
import { getRequestHeaders } from "@tanstack/react-start/server";
import { useState } from "react";
import { z } from "zod";
import AuthLayout from "@/components/(auth)/auth-layout";
import ErrorComponent from "@/components/common/error-component";
import NotFound from "@/components/common/not-found";
import { getAuth } from "@/lib/auth.server";
import { LinkSquareIcon } from "@/lib/icons";
import {
  OAUTH_LOGIN_PAGE,
  OIDC_STANDARD_SCOPES,
  SCOPE_DESCRIPTIONS,
} from "@/lib/oauth-scopes";
import { captureException } from "@/lib/sentry";
import { authMiddleware } from "@/middlewares/auth";
import { type ClientInfo, resolveRedirectHost } from "./consent-utils";

type ScopeLevel = "read" | "write" | "admin";

const LEVEL_VARIANTS: Record<ScopeLevel, NonNullable<BadgeProps["variant"]>> = {
  read: "info-light",
  write: "warning-light",
  admin: "destructive-light",
};

const HIDDEN_SCOPES = new Set<string>(OIDC_STANDARD_SCOPES);

function buildSearchParams(search: Record<string, string | undefined>): string {
  const params = new URLSearchParams();
  for (const [key, value] of Object.entries(search)) {
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

const optionalSearchParam = z.string().optional().catch(undefined);

const consentSearchSchema = z
  .object({
    client_id: optionalSearchParam,
    scope: optionalSearchParam,
    redirect_uri: optionalSearchParam,
    state: optionalSearchParam,
    response_type: optionalSearchParam,
    code_challenge: optionalSearchParam,
    code_challenge_method: optionalSearchParam,
    exp: optionalSearchParam,
    sig: optionalSearchParam,
    nonce: optionalSearchParam,
    prompt: optionalSearchParam,
    request_uri: optionalSearchParam,
    max_age: optionalSearchParam,
    login_hint: optionalSearchParam,
    acr_values: optionalSearchParam,
  })
  .catchall(optionalSearchParam);

const fetchClientInfo = createServerFn({ method: "GET" })
  .inputValidator(z.object({ clientId: z.string().min(1) }))
  .middleware([authMiddleware])
  .handler(async ({ data }) => {
    try {
      const client = await ((await getAuth()).api as any).getOAuthClientPublic({
        query: { client_id: data.clientId },
        headers: getRequestHeaders(),
      });
      if (!client) {
        return null;
      }
      return {
        name: (client as any).client_name ?? "Unknown Application",
        clientId: (client as any).client_id ?? data.clientId,
        redirectUrls: (client as any).redirect_uris ?? [],
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
    const result = await (await getAuth()).api.oauth2Consent({
      body: {
        accept: data.accept,
        scope: data.scope,
        oauth_query: data.oauthQuery,
      },
      headers: getRequestHeaders(),
    });
    return result;
  });

export const Route = createFileRoute("/(auth)/oauth/consent")({
  validateSearch: consentSearchSchema,
  beforeLoad: ({ context, location }) => {
    if (!context.isAuthenticated) {
      throw redirect({
        to: OAUTH_LOGIN_PAGE,
        search: {
          redirect: `/oauth/consent${location.searchStr}`,
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
  const oauthQuery =
    typeof window === "undefined"
      ? buildSearchParams(search as Record<string, string | undefined>)
      : window.location.search.slice(1);

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
  const redirectHost = resolveRedirectHost(clientInfo, search.redirect_uri);

  return (
    <AuthLayout
      description={`"${clientName}" wants access to your Strait account`}
      title="Authorize Application"
    >
      <div className="flex flex-col gap-4">
        {/* Client warning for unrecognized clients */}
        {clientError || !clientInfo ? (
          <Alert variant="warning">
            <AlertDescription>
              Unable to verify this application. Proceed with caution.
            </AlertDescription>
          </Alert>
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
        {redirectHost ? (
          <Alert>
            <HugeiconsIcon
              className="size-3.5 shrink-0 text-muted-foreground"
              icon={LinkSquareIcon}
            />
            <AlertDescription className="truncate">
              Redirects to{" "}
              <span className="font-medium text-foreground">
                {redirectHost}
              </span>
            </AlertDescription>
          </Alert>
        ) : null}

        {/* Error display */}
        {error ? (
          <Alert variant="destructive">
            <AlertDescription>{error}</AlertDescription>
          </Alert>
        ) : null}

        {/* Action buttons */}
        <div className="flex w-full gap-3">
          <Button
            className="flex-1"
            disabled={status === "authorizing" || status === "denying"}
            onClick={() => handleConsent(false)}
            type="button"
            variant="secondary-outline"
          >
            {status === "denying" ? "Denying..." : "Deny"}
          </Button>
          <Button
            className="flex-1"
            disabled={status === "authorizing" || status === "denying"}
            onClick={() => handleConsent(true)}
            type="button"
            variant="brand-solid"
          >
            {status === "authorizing" ? (
              <>
                <Spinner />
                Authorizing...
              </>
            ) : (
              "Authorize"
            )}
          </Button>
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
    <Frame stacked>
      <FramePanel>
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
                <Badge radius="md" size="xs" variant="secondary-light">
                  other
                </Badge>
              </div>
              <div className="mt-1.5 flex flex-col gap-1 pl-1">
                {unknownScopes.map((scope) => (
                  <span className="text-muted-foreground text-sm" key={scope}>
                    {scope}
                  </span>
                ))}
              </div>
            </div>
          ) : null}
        </div>
      </FramePanel>
      <FramePanel>
        <p className="text-muted-foreground text-xs">
          This app will <span className="font-medium text-foreground">not</span>{" "}
          be able to manage API keys, change account settings, or access billing
          information.
        </p>
      </FramePanel>
    </Frame>
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
        <Spinner className="text-primary" size="lg" />
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
  const labels: Record<ScopeLevel, string> = {
    read: "Read",
    write: "Write",
    admin: "Admin",
  };

  return (
    <div>
      <div className="flex items-center gap-1.5">
        <Badge radius="md" size="xs" variant={LEVEL_VARIANTS[level]}>
          {labels[level]}
        </Badge>
      </div>
      <div className="mt-1.5 flex flex-col gap-1 pl-1">
        {scopes.map((scope) => {
          const info = SCOPE_DESCRIPTIONS[scope];
          if (!info) {
            return null;
          }
          return (
            <div className="flex flex-col" key={scope}>
              <Badge
                className="w-fit"
                dot
                radius="md"
                size="xs"
                variant={LEVEL_VARIANTS[level]}
              >
                {info.label}
              </Badge>
              <div className="flex flex-col">
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
