import { spawn } from "node:child_process";
import fs from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import pg from "pg";
import { signInAndSaveState } from "./setup/auth";
import {
  ensureLimitedMemberExists,
  ensureOrgExists,
  ensureProjectExists,
  ensureProjectRbacExists,
  ensureUserExists,
} from "./setup/db";
import { ensureUnlimitedE2EPlan } from "./support/auth-db";
import { loadE2EEnv } from "./support/env";
import {
  fakeEndpointContextPath,
  writeFakeEndpointContext,
  writeRunContext,
} from "./support/run-context";

const __filename = fileURLToPath(import.meta.url);
const __dirname = dirname(__filename);
const fakeEndpointServerPath = resolve(
  __dirname,
  "support/fake-endpoint-server.mjs"
);

export default async function globalSetup() {
  loadE2EEnv();

  const email = process.env.E2E_USER_EMAIL;
  const password = process.env.E2E_USER_PASSWORD;
  const limitedEmail =
    process.env.E2E_LIMITED_USER_EMAIL ?? "e2e-limited@example.com";
  const limitedPassword =
    process.env.E2E_LIMITED_USER_PASSWORD ?? password ?? "password123456";
  const authDbUrl = process.env.AUTH_DATABASE_URL;
  const baseURL = process.env.EXPECT_BASE_URL || "http://localhost:5173";
  const apiURL = process.env.STRAIT_API_URL || "http://localhost:8080";
  const internalSecret = process.env.INTERNAL_SECRET || "";
  const fakeEndpointUrl = await ensureFakeEndpoint();

  if (!(email && password)) {
    throw new Error(
      "E2E_USER_EMAIL and E2E_USER_PASSWORD must be set for e2e tests"
    );
  }

  // Trigger Better Auth schema initialization
  await fetch(`${baseURL}/api/auth/session`, {
    headers: { Origin: baseURL },
  }).catch(() => {
    // Expected -- triggers table creation
  });
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
      const projectId = await ensureProjectExists(
        pool,
        user.id,
        orgId,
        user.activeProjectId,
        apiURL,
        internalSecret
      );
      const limitedUser = await ensureLimitedMemberExists(
        pool,
        limitedEmail,
        limitedPassword,
        baseURL,
        orgId,
        projectId
      );
      await ensureProjectRbacExists(pool, projectId, user.id, limitedUser.id);
      await ensureUnlimitedE2EPlan(orgId);

      fs.mkdirSync("playwright/.auth", { recursive: true });
      fs.writeFileSync(
        "playwright/.auth/project.json",
        JSON.stringify({
          projectId,
          orgId,
          userId: user.id,
          limitedUserId: limitedUser.id,
          limitedUserEmail: limitedEmail,
          fakeEndpointUrl,
        })
      );
      writeRunContext({
        projectId,
        orgId,
        userId: user.id,
        limitedUserId: limitedUser.id,
        limitedUserEmail: limitedEmail,
        fakeEndpointUrl,
      });
    } finally {
      await pool.end();
    }
  }

  await signInAndSaveState(baseURL, email, password);
  await signInAndSaveState(
    baseURL,
    limitedEmail,
    limitedPassword,
    "playwright/.auth/limited-user.json"
  );
}

async function ensureFakeEndpoint() {
  if (process.env.E2E_FAKE_ENDPOINT_URL) {
    writeFakeEndpointContext({
      url: process.env.E2E_FAKE_ENDPOINT_URL,
      managed: false,
    });
    return process.env.E2E_FAKE_ENDPOINT_URL;
  }

  fs.rmSync(fakeEndpointContextPath, { force: true });
  const child = spawn(process.execPath, [fakeEndpointServerPath], {
    detached: true,
    env: {
      ...process.env,
      E2E_FAKE_ENDPOINT_INFO_PATH: fakeEndpointContextPath,
    },
    stdio: "ignore",
  });
  child.unref();

  const context = await waitForFakeEndpoint();
  await waitForHealth(context.url);
  return context.url;
}

async function waitForFakeEndpoint() {
  const deadline = Date.now() + 10_000;
  while (Date.now() < deadline) {
    try {
      const raw = fs.readFileSync(fakeEndpointContextPath, "utf-8");
      const context = JSON.parse(raw) as { url: string; pid?: number };
      if (context.url) {
        writeFakeEndpointContext({ ...context, managed: true });
        return context;
      }
    } catch {
      // Wait for the child process to write its runtime context.
    }
    await new Promise((resolve) => setTimeout(resolve, 100));
  }
  throw new Error("Timed out waiting for e2e fake endpoint server");
}

async function waitForHealth(url: string) {
  const deadline = Date.now() + 10_000;
  while (Date.now() < deadline) {
    try {
      const response = await fetch(`${url}/health`);
      if (response.ok) {
        return;
      }
    } catch {
      // Keep polling until the fake endpoint accepts connections.
    }
    await new Promise((resolve) => setTimeout(resolve, 100));
  }
  throw new Error(`Timed out waiting for fake endpoint health at ${url}`);
}
