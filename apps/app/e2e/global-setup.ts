import fs from "node:fs";
import pg from "pg";
import { migrateAuthDatabase } from "../scripts/lib/local-bootstrap";
import { signInAndSaveState } from "./setup/auth";
import {
  ensureOrgExists,
  ensureProjectExists,
  ensureUserExists,
} from "./setup/db";

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

  if (authDbUrl) {
    process.env.AUTH_DATABASE_URL = authDbUrl;
  }
  process.env.BETTER_AUTH_URL ||= baseURL;
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
