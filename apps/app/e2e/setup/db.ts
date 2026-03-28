import type pg from "pg";
import {
  cleanupAuthUser,
  ensureOrganizationExists,
  ensurePasswordUserExists,
  ensureProjectExists as ensureProjectForUser,
} from "../../scripts/lib/local-auth-user";

const E2E_USER_NAME = "E2E Test User";

export function ensureUserExists(
  pool: pg.Pool,
  email: string,
  password: string,
  baseURL: string
) {
  return ensurePasswordUserExists(pool, {
    email,
    password,
    baseURL,
    name: E2E_USER_NAME,
  });
}

export function ensureOrgExists(
  pool: pg.Pool,
  userId: string,
  existingOrgId: string | null
) {
  return ensureOrganizationExists(pool, {
    userId,
    existingOrgId,
    workspaceName: `${E2E_USER_NAME}'s Workspace`,
  });
}

export function ensureProjectExists(
  pool: pg.Pool,
  userId: string,
  orgId: string,
  existingProjectId: string | null,
  apiURL: string,
  internalSecret: string
) {
  return ensureProjectForUser(pool, {
    userId,
    orgId,
    existingProjectId,
    apiURL,
    internalSecret,
    projectName: "Default Project",
  });
}

export async function cleanupTestUser(authDbUrl: string, email: string) {
  await cleanupAuthUser(authDbUrl, email);
}
