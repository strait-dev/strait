import { cleanupTestUser } from "./setup/db";

export default async function globalTeardown() {
  const email = process.env.E2E_USER_EMAIL;
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
