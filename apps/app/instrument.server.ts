import * as Sentry from "@sentry/tanstackstart-react";

const dsn = process.env.VITE_SENTRY_DSN;

if (dsn) {
  Sentry.init({
    dsn,
    environment: process.env.VITE_SENTRY_ENVIRONMENT || "production",
    sendDefaultPii: true,
    enableLogs: true,
    tracesSampleRate: 1.0,
    profilesSampleRate: 1.0,
  });
}
