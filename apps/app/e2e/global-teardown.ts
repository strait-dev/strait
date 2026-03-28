import { applyLocalDefaults } from "../scripts/lib/local-bootstrap";
import { cleanupTestUser } from "./setup/db";

const DEFAULT_E2E_USER_EMAIL = "e2e@local.strait";

export default async function globalTeardown() {
  Object.assign(process.env, applyLocalDefaults(process.env, []));

  const email = process.env.E2E_USER_EMAIL ?? DEFAULT_E2E_USER_EMAIL;
  const authDbUrl = process.env.AUTH_DATABASE_URL;

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
