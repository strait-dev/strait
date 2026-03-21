import * as Sentry from "@sentry/tanstackstart-react";
import type { AnyRouter } from "@tanstack/react-router";
import type { AuthUser } from "@/routes/__root";

/**
 * Re-exported Sentry APIs for use across the application.
 * All consumer files should import from `@/lib/sentry` instead of `@sentry/react` directly.
 */
export const captureException = Sentry.captureException.bind(Sentry);

const ANDROID_WEBOS_IPHONE_IPAD_IPOD_BLACKBERRY_IEMOBILE_OPERA_MINI_REGEX =
  /Android|webOS|iPhone|iPad|iPod|BlackBerry|IEMobile|Opera Mini/i;

const MAX_USER_AGENT_LENGTH = 200;

/**
 * Initialize Sentry for client-side error tracking and performance monitoring.
 * Disabled in development to keep dashboards clean.
 */
export function initializeSentry(router: AnyRouter) {
  const isDevelopment = import.meta.env.DEV;

  // Skip Sentry initialization in development to keep dashboards clean
  if (isDevelopment) {
    return;
  }

  // Initialize Sentry on client-side only when DSN is available
  if (!router.isServer) {
    const sentryDsn = import.meta.env.VITE_SENTRY_DSN;
    const sentryEnvironment = import.meta.env.VITE_SENTRY_ENVIRONMENT;
    const isProduction = import.meta.env.PROD;

    Sentry.init({
      dsn: sentryDsn,
      // Use explicit env var if set, otherwise infer from Vite's PROD flag
      environment:
        sentryEnvironment || (isProduction ? "production" : "development"),
      // Adds request headers and IP for users, for more info visit:
      // https://docs.sentry.io/platforms/javascript/guides/tanstackstart-react/configuration/options/#sendDefaultPii
      sendDefaultPii: true,
      integrations: [
        // performance
        Sentry.tanstackRouterBrowserTracingIntegration(router),
        // performance
        // session-replay
        Sentry.replayIntegration(),
        // session-replay
        // browser profiling
        Sentry.browserProfilingIntegration(),
        // browser profiling
        // logging
        // send console.log, console.warn, and console.error calls as logs to Sentry
        Sentry.consoleLoggingIntegration({
          levels: ["log", "warn", "error"],
        }),
        // logging
        // user-feedback
        Sentry.feedbackIntegration({
          // Additional SDK configuration goes in here, for example:
          colorScheme: "system",
        }),
        // user-feedback
      ],
      // logs
      // Enable logs to be sent to Sentry (enabled for both production and dev test page)
      enableLogs: true,
      // logs
      // performance
      // Set tracesSampleRate to 1.0 to capture 100%
      // of transactions for tracing.
      // We recommend adjusting this value in production.
      // Learn more at https://docs.sentry.io/platforms/javascript/configuration/options/#traces-sample-rate
      tracesSampleRate: 1.0,
      // performance
      // session-replay
      // Capture Replay for 10% of all sessions,
      // plus for 100% of sessions with an error.
      // Learn more at https://docs.sentry.io/platforms/javascript/session-replay/configuration/#general-integration-configuration
      replaysSessionSampleRate: 0.1,
      replaysOnErrorSampleRate: 1.0,
      // session-replay
      // browser profiling
      // Set profilesSampleRate to 1.0 to profile every transaction.
      // Since profilesSampleRate is relative to tracesSampleRate,
      // the final profiling rate can be computed as tracesSampleRate * profilesSampleRate
      // For example, a tracesSampleRate of 0.5 and profilesSampleRate of 0.5 would
      // results in 25% of transactions being profiled (0.5*0.5=0.25)
      profilesSampleRate: 1.0,
      // browser profiling
    });
  }
}

/**
 * Sets essential user context in Sentry for error tracking
 * Only includes information necessary for debugging errors
 */
export function setSentryUser(session: { user: AuthUser } | null) {
  if (!session?.user) {
    // Clear user context when no session or user
    Sentry.setUser(null);
    return;
  }

  const user = session.user;

  // Set essential user context for error debugging
  Sentry.setUser({
    id: user.id,
    email: user.email,
    username: user.name || user.email,
    // Organization context is critical for multi-tenant error debugging
    ...(user.defaultOrganizationId && {
      organization_id: user.defaultOrganizationId,
    }),
  });

  // Only set tags that are useful for error filtering and prioritization
  if (user.defaultOrganizationId) {
    Sentry.setTag("organization_id", user.defaultOrganizationId);
  }
}

/**
 * Clears user context in Sentry when user logs out
 */
export function clearSentryUser() {
  Sentry.setUser(null);
  // Clear organization tag
  Sentry.setTag("organization_id", null);
}

/**
 * Captures authentication errors with essential context for debugging
 */
export function captureSentryAuthError(
  error: unknown,
  context: {
    operation:
      | "signin"
      | "signup"
      | "email-signin"
      | "magic-link"
      | "passkey"
      | "sso"
      | "2fa-verify"
      | "password-reset"
      | "password-reset-request"
      | "accept-invitation"
      | "google-oauth";
    email?: string;
    provider?: "google" | "github" | "passkey" | "magic-link" | "sso" | "email";
    invitationId?: string;
  }
) {
  // Set tags for filtering in Sentry dashboard
  Sentry.setTag("auth.operation", context.operation);
  Sentry.setTag("auth.provider", context.provider || "google");

  if (context.invitationId) {
    Sentry.setTag("auth.has_invitation", true);
  }

  // Get user agent for additional debugging context
  const userAgent =
    typeof navigator === "undefined" ? undefined : navigator.userAgent;
  const isMobile = userAgent
    ? ANDROID_WEBOS_IPHONE_IPAD_IPOD_BLACKBERRY_IEMOBILE_OPERA_MINI_REGEX.test(
        userAgent
      )
    : false;

  // Capture the error with context
  Sentry.captureException(error, {
    extra: {
      operation: context.operation,
      provider: context.provider || "email",
      email_domain: context.email ? context.email.split("@")[1] : undefined,
      has_invitation: !!context.invitationId,
      is_mobile: isMobile,
      browser_info: userAgent
        ? {
            is_mobile: isMobile,
            user_agent: userAgent.slice(0, MAX_USER_AGENT_LENGTH), // Truncate for privacy
          }
        : undefined,
    },
  });
}
