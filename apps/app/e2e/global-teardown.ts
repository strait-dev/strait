import { cleanupTestUser } from "./setup/db";
import { loadE2EEnv } from "./support/env";
import { readFakeEndpointContext } from "./support/run-context";

export default async function globalTeardown() {
  loadE2EEnv();

  const email = process.env.E2E_USER_EMAIL;
  const authDbUrl = process.env.AUTH_DATABASE_URL;

  stopManagedFakeEndpoint();

  if (!(email && authDbUrl)) {
    return;
  }

  try {
    await cleanupTestUser(authDbUrl, email);
  } catch (err) {
    // Teardown should never fail the test run
    console.warn("Teardown warning:", err instanceof Error ? err.message : err);
  }
}

function stopManagedFakeEndpoint() {
  const context = readFakeEndpointContext();
  if (!(context?.managed && context.pid)) {
    return;
  }

  try {
    process.kill(context.pid, "SIGTERM");
  } catch {
    // The endpoint may already be gone if Playwright was interrupted.
  }
}
