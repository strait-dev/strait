import { chromium, type FullConfig } from "@playwright/test";
import pg from "pg";

const E2E_USER_NAME = "E2E Test User";

export default async function globalSetup(config: FullConfig) {
  const email = process.env.E2E_USER_EMAIL;
  const password = process.env.E2E_USER_PASSWORD;
  const authDbUrl = process.env.AUTH_DATABASE_URL;

  if (!(email && password)) {
    throw new Error(
      "E2E_USER_EMAIL and E2E_USER_PASSWORD must be set for e2e tests"
    );
  }

  const baseURL =
    config.projects[0]?.use?.baseURL ?? "http://localhost:5173";

  // Step 1: Try to sign up the test user via the UI
  const browser = await chromium.launch();
  const context = await browser.newContext();
  const page = await context.newPage();

  try {
    // Attempt signup -- if user exists, this will fail gracefully
    await page.goto(`${baseURL}/signup`);
    await page.getByLabel("Name").fill(E2E_USER_NAME);
    await page.getByLabel("Email").fill(email);
    await page.getByLabel("Password").fill(password);
    await page.getByRole("button", { name: "Create account" }).click();

    // Wait for either success (email sent) or error (user exists)
    await page.waitForTimeout(2000);
  } catch {
    // Signup may fail if user already exists -- that's fine
  }

  // Step 2: Bypass email verification via direct DB update
  if (authDbUrl) {
    const pool = new pg.Pool({ connectionString: authDbUrl });
    try {
      await pool.query(
        `UPDATE "user" SET "emailVerified" = true WHERE "email" = $1`,
        [email]
      );
    } finally {
      await pool.end();
    }
  }

  // Step 3: Sign in and save storageState
  await page.goto(`${baseURL}/login`);
  await page.getByLabel("Email").fill(email);
  await page.getByLabel("Password").fill(password);
  await page.getByRole("button", { name: "Sign in" }).click();

  // Wait for redirect to /app
  await page.waitForURL("**/app/**", { timeout: 15_000 });

  // Save authenticated state
  await context.storageState({ path: "playwright/.auth/user.json" });

  await browser.close();
}
