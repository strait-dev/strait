import { chromium } from "@playwright/test";
import pg from "pg";

const E2E_USER_NAME = "E2E Test User";

export default async function globalSetup() {
  const email = process.env.E2E_USER_EMAIL;
  const password = process.env.E2E_USER_PASSWORD;
  const authDbUrl = process.env.AUTH_DATABASE_URL;

  if (!(email && password)) {
    throw new Error(
      "E2E_USER_EMAIL and E2E_USER_PASSWORD must be set for e2e tests"
    );
  }

  const baseURL = process.env.EXPECT_BASE_URL || "http://localhost:5173";

  // Step 1: Ensure user exists and email is verified
  if (authDbUrl) {
    const pool = new pg.Pool({ connectionString: authDbUrl });
    try {
      const existing = await pool.query(
        `SELECT "id" FROM "user" WHERE "email" = $1`,
        [email]
      );

      if (existing.rows.length === 0) {
        // User doesn't exist -- create via Better Auth API
        const signupRes = await fetch(`${baseURL}/api/auth/sign-up/email`, {
          method: "POST",
          headers: { "Content-Type": "application/json", Origin: baseURL },
          body: JSON.stringify({ name: E2E_USER_NAME, email, password }),
        });
        if (!signupRes.ok) {
          const text = await signupRes.text();
          console.warn(`Signup returned ${signupRes.status}: ${text}`);
        }
      }

      // Bypass email verification
      await pool.query(
        `UPDATE "user" SET "emailVerified" = true WHERE "email" = $1`,
        [email]
      );
    } finally {
      await pool.end();
    }
  }

  // Step 2: Sign in via Better Auth API and save storageState
  const signinRes = await fetch(`${baseURL}/api/auth/sign-in/email`, {
    method: "POST",
    headers: { "Content-Type": "application/json", Origin: baseURL },
    body: JSON.stringify({ email, password }),
    redirect: "manual",
  });

  // Extract session cookie from the sign-in response
  const setCookies = signinRes.headers.getSetCookie();
  const sessionCookie = setCookies.find((c) =>
    c.startsWith("better-auth.session_token=")
  );

  if (!sessionCookie) {
    const body = await signinRes.text();
    throw new Error(
      `Sign-in failed (${signinRes.status}): no session cookie returned. Body: ${body.slice(0, 200)}`
    );
  }

  // Parse the cookie value
  const tokenMatch = sessionCookie.match(/better-auth\.session_token=([^;]+)/);
  if (!tokenMatch) {
    throw new Error("Could not parse session token from cookie");
  }

  // Build storageState with the session cookie
  const url = new URL(baseURL);
  const storageState = {
    cookies: [
      {
        name: "better-auth.session_token",
        value: tokenMatch[1],
        domain: url.hostname,
        path: "/",
        expires: -1,
        httpOnly: true,
        secure: false,
        sameSite: "Lax" as const,
      },
    ],
    origins: [],
  };

  // Verify the session works by loading a page
  const browser = await chromium.launch();
  const context = await browser.newContext({ storageState });
  const page = await context.newPage();

  try {
    await page.goto(`${baseURL}/app/dashboard`);
    await page.waitForURL("**/app/**", { timeout: 15_000 });
    // Save the full storageState (includes any additional cookies set by the app)
    await context.storageState({ path: "playwright/.auth/user.json" });
  } finally {
    await browser.close();
  }
}
