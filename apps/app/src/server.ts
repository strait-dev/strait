import "../instrument.server";
import { wrapFetchWithSentry } from "@sentry/tanstackstart-react";
import handler, { createServerEntry } from "@tanstack/react-start/server-entry";
import { handleWellKnownOAuthRequest } from "@/lib/well-known-oauth.server";

/**
 * One-shot boot log: record which Postgres path the auth layer is using.
 */
let authDbSourceLogged = false;
const logAuthDbSource = () => {
  if (authDbSourceLogged) {
    return;
  }
  authDbSourceLogged = true;
  console.log(
    JSON.stringify({
      msg: "auth_db_source",
      source: "AUTH_DATABASE_URL",
    })
  );
};

export default createServerEntry(
  wrapFetchWithSentry({
    async fetch(request: Request) {
      logAuthDbSource();
      const wellKnownResponse = await handleWellKnownOAuthRequest(request);
      if (wellKnownResponse) {
        return wellKnownResponse;
      }
      return handler.fetch(request);
    },
  })
);
