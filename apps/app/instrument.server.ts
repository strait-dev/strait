import * as Sentry from "@sentry/tanstackstart-react";
import {
  scrubSentryBreadcrumb,
  scrubSentryEvent,
} from "./src/lib/sentry-scrub";

const dsn = process.env.VITE_SENTRY_DSN;

if (dsn) {
  Sentry.init({
    dsn,
    environment: process.env.VITE_SENTRY_ENVIRONMENT || "production",
    sendDefaultPii: false,
    beforeSend: scrubSentryEvent,
    beforeBreadcrumb: scrubSentryBreadcrumb,
    enableLogs: true,
    tracesSampleRate: 1.0,
    profilesSampleRate: 1.0,
  });
}
