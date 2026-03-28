import fs from "node:fs";
import pg from "pg";
import {
  applyLocalDefaults,
  migrateAuthDatabase,
} from "../scripts/lib/local-bootstrap";
import { signInAndSaveState } from "./setup/auth";
import {
  ensureOrgExists,
  ensureProjectExists,
  ensureUserExists,
} from "./setup/db";

const DEFAULT_E2E_USER_EMAIL = "e2e@local.strait";
const DEFAULT_E2E_USER_PASSWORD = "e2epassword123";

export default async function globalSetup() {
  const env = applyLocalDefaults(process.env, []);
  const baseURL = process.env.EXPECT_BASE_URL || "http://localhost:5173";

  Object.assign(process.env, env, {
    BETTER_AUTH_URL: process.env.BETTER_AUTH_URL || baseURL,
    VITE_BASE_URL: process.env.VITE_BASE_URL || baseURL,
  });

  const email = process.env.E2E_USER_EMAIL ?? DEFAULT_E2E_USER_EMAIL;
  const password =
    process.env.E2E_USER_PASSWORD ?? DEFAULT_E2E_USER_PASSWORD;
  const authDbUrl = process.env.AUTH_DATABASE_URL;
  const apiURL = process.env.STRAIT_API_URL || "http://localhost:8080";
  const internalSecret = process.env.INTERNAL_SECRET || "";

  process.env.E2E_USER_EMAIL = email;
  process.env.E2E_USER_PASSWORD = password;

  await migrateAuthDatabase();

  if (authDbUrl) {
    const pool = new pg.Pool({ connectionString: authDbUrl });
    try {
      const user = await ensureUserExists(pool, email, password, baseURL);
      const orgId = await ensureOrgExists(
        pool,
        user.id,
        user.defaultOrganizationId
      );
      const projectId = await ensureProjectExists(
        pool,
        user.id,
        orgId,
        user.activeProjectId,
        apiURL,
        internalSecret
      );

      fs.mkdirSync("playwright/.auth", { recursive: true });
      fs.writeFileSync(
        "playwright/.auth/project.json",
        JSON.stringify({ projectId, orgId, userId: user.id })
      );
    } finally {
      await pool.end();
    }
  }

  await signInAndSaveState(baseURL, email, password);
}
