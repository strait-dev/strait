import crypto from "node:crypto";
import { chromium } from "@playwright/test";
import pg from "pg";

const E2E_USER_NAME = "E2E Test User";

async function ensureUserExists(
  pool: pg.Pool,
  email: string,
  password: string,
  baseURL: string
) {
  let userRow = await pool.query(
    `SELECT "id", "defaultOrganizationId", "activeProjectId" FROM "user" WHERE "email" = $1`,
    [email]
  );

  if (userRow.rows.length === 0) {
    const signupRes = await fetch(`${baseURL}/api/auth/sign-up/email`, {
      method: "POST",
      headers: { "Content-Type": "application/json", Origin: baseURL },
      body: JSON.stringify({ name: E2E_USER_NAME, email, password }),
    });
    if (!signupRes.ok) {
      console.warn(
        `Signup returned ${signupRes.status}: ${await signupRes.text()}`
      );
    }

    userRow = await pool.query(
      `SELECT "id", "defaultOrganizationId", "activeProjectId" FROM "user" WHERE "email" = $1`,
      [email]
    );
  }

  if (userRow.rows.length === 0) {
    throw new Error("Failed to create test user");
  }

  await pool.query(`UPDATE "user" SET "emailVerified" = true WHERE "id" = $1`, [
    userRow.rows[0].id,
  ]);

  return userRow.rows[0];
}

async function ensureOrgExists(
  pool: pg.Pool,
  userId: string,
  existingOrgId: string | null
) {
  if (existingOrgId) {
    return existingOrgId;
  }

  const orgId = crypto.randomUUID();
  const orgSlug = `ws-${crypto.randomUUID().slice(0, 12)}`;

  await pool.query(
    `INSERT INTO "organization" ("id", "name", "slug", "createdAt")
     VALUES ($1, $2, $3, NOW()) ON CONFLICT DO NOTHING`,
    [orgId, `${E2E_USER_NAME}'s Workspace`, orgSlug]
  );

  await pool.query(
    `INSERT INTO "member" ("id", "organizationId", "userId", "role", "createdAt")
     VALUES ($1, $2, $3, 'owner', NOW()) ON CONFLICT DO NOTHING`,
    [crypto.randomUUID(), orgId, userId]
  );

  await pool.query(
    `UPDATE "user" SET "defaultOrganizationId" = $1 WHERE "id" = $2`,
    [orgId, userId]
  );

  return orgId;
}

async function ensureProjectExists(
  pool: pg.Pool,
  userId: string,
  orgId: string,
  existingProjectId: string | null,
  apiURL: string,
  internalSecret: string
) {
  if (existingProjectId) {
    return existingProjectId;
  }

  const projectId = crypto.randomUUID();
  const projectSlug = `project-${projectId.slice(0, 8)}`;

  await pool.query(`
    CREATE TABLE IF NOT EXISTS project (
      id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
      organization_id TEXT NOT NULL,
      name TEXT NOT NULL,
      slug TEXT NOT NULL,
      description TEXT DEFAULT '',
      created_by TEXT NOT NULL,
      created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
      updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
      UNIQUE(organization_id, slug)
    )
  `);

  await pool.query(
    `INSERT INTO project (id, organization_id, name, slug, created_by)
     VALUES ($1, $2, $3, $4, $5) ON CONFLICT DO NOTHING`,
    [projectId, orgId, "Default Project", projectSlug, userId]
  );

  await pool.query(`UPDATE "user" SET "activeProjectId" = $1 WHERE "id" = $2`, [
    projectId,
    userId,
  ]);

  // Sync project to Go API (best-effort)
  if (internalSecret) {
    try {
      await fetch(`${apiURL}/v1/projects`, {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          "X-Internal-Secret": internalSecret,
        },
        body: JSON.stringify({
          id: projectId,
          org_id: orgId,
          name: "Default Project",
        }),
      });
    } catch {
      // Best-effort sync
    }
  }

  return projectId;
}

async function signInAndSaveState(
  baseURL: string,
  email: string,
  password: string
) {
  // Sign in via API and get the session cookie
  const signinRes = await fetch(`${baseURL}/api/auth/sign-in/email`, {
    method: "POST",
    headers: { "Content-Type": "application/json", Origin: baseURL },
    body: JSON.stringify({ email, password }),
    redirect: "manual",
  });

  const setCookies = signinRes.headers.getSetCookie();
  const sessionCookie = setCookies.find((c) =>
    c.startsWith("better-auth.session_token=")
  );

  if (!sessionCookie) {
    const body = await signinRes.text();
    throw new Error(
      `Sign-in failed (${signinRes.status}): no session cookie. Body: ${body.slice(0, 200)}`
    );
  }

  const tokenMatch = sessionCookie.match(/better-auth\.session_token=([^;]+)/);
  if (!tokenMatch) {
    throw new Error("Could not parse session token from cookie");
  }

  // Set active organization via API
  const cookieHeader = `better-auth.session_token=${tokenMatch[1]}`;
  const sessionRes = await fetch(`${baseURL}/api/auth/session`, {
    headers: { Cookie: cookieHeader, Origin: baseURL },
  });
  if (sessionRes.ok) {
    const session = (await sessionRes.json()) as {
      user?: { defaultOrganizationId?: string };
    };
    const orgId = session?.user?.defaultOrganizationId;
    if (orgId) {
      await fetch(`${baseURL}/api/auth/organization/set-active`, {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          Cookie: cookieHeader,
          Origin: baseURL,
        },
        body: JSON.stringify({ organizationId: orgId }),
      });
    }
  }

  // Create browser context with the session cookie and navigate to verify
  const browser = await chromium.launch();
  const context = await browser.newContext();

  // Add the cookie to the browser context
  await context.addCookies([
    {
      name: "better-auth.session_token",
      value: tokenMatch[1],
      domain: "localhost",
      path: "/",
      httpOnly: true,
      secure: false,
      sameSite: "Lax",
    },
  ]);

  const page = await context.newPage();

  try {
    await page.goto(`${baseURL}/app/dashboard`);
    await page.waitForURL("**/app/**", { timeout: 15_000 });
    await context.storageState({ path: "playwright/.auth/user.json" });
  } finally {
    await browser.close();
  }
}

export default async function globalSetup() {
  const email = process.env.E2E_USER_EMAIL;
  const password = process.env.E2E_USER_PASSWORD;
  const authDbUrl = process.env.AUTH_DATABASE_URL;
  const baseURL = process.env.EXPECT_BASE_URL || "http://localhost:5173";
  const apiURL = process.env.STRAIT_API_URL || "http://localhost:8080";
  const internalSecret = process.env.INTERNAL_SECRET || "";

  if (!(email && password)) {
    throw new Error(
      "E2E_USER_EMAIL and E2E_USER_PASSWORD must be set for e2e tests"
    );
  }

  // Trigger Better Auth schema initialization by hitting the app
  // Better Auth creates its tables (user, session, account, etc.) on first request
  await fetch(`${baseURL}/api/auth/session`, {
    headers: { Origin: baseURL },
  }).catch(() => {
    // Expected to fail -- we just need the request to trigger schema creation
  });
  // Give it a moment to finish table creation
  await new Promise((r) => setTimeout(r, 2000));

  if (authDbUrl) {
    const pool = new pg.Pool({ connectionString: authDbUrl });
    try {
      const user = await ensureUserExists(pool, email, password, baseURL);
      const orgId = await ensureOrgExists(
        pool,
        user.id,
        user.defaultOrganizationId
      );
      await ensureProjectExists(
        pool,
        user.id,
        orgId,
        user.activeProjectId,
        apiURL,
        internalSecret
      );
    } finally {
      await pool.end();
    }
  }

  await signInAndSaveState(baseURL, email, password);
}
