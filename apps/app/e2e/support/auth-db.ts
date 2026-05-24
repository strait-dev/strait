import crypto from "node:crypto";
import pg from "pg";
import type { ApiHelper } from "../fixtures/api";
import { loadE2EEnv } from "./env";
import { readRunContext } from "./run-context";

export type IsolatedOrgProject = {
  orgId: string;
  projectId: string;
  projectName: string;
};

async function withAuthPool<T>(fn: (pool: pg.Pool) => Promise<T>) {
  loadE2EEnv();
  const authDbUrl = process.env.AUTH_DATABASE_URL;
  if (!authDbUrl) {
    throw new Error("AUTH_DATABASE_URL is required");
  }

  const pool = new pg.Pool({ connectionString: authDbUrl });
  try {
    return await fn(pool);
  } finally {
    await pool.end();
  }
}

/** Give a local e2e org enough backend quota for full-suite browser runs. */
export async function ensureUnlimitedE2EPlan(orgId: string) {
  loadE2EEnv();
  const databaseUrl = process.env.DATABASE_URL ?? process.env.AUTH_DATABASE_URL;
  if (!databaseUrl) {
    return;
  }

  const pool = new pg.Pool({ connectionString: databaseUrl });
  try {
    await pool.query(
      `
        INSERT INTO organization_subscriptions (
          id,
          org_id,
          plan_tier,
          status,
          entitlements,
          created_at,
          updated_at
        )
        VALUES ($1, $2, 'enterprise', 'active', '{}'::jsonb, NOW(), NOW())
        ON CONFLICT (org_id) DO UPDATE SET
          plan_tier = 'enterprise',
          status = 'active',
          entitlements = '{}'::jsonb,
          updated_at = NOW()
      `,
      [crypto.randomUUID(), orgId]
    );
  } finally {
    await pool.end();
  }
}

/** Point the e2e user's active project at a specific project id. */
export async function setE2EUserActiveProjectId(projectId: string | null) {
  const userId = readRunContext()?.userId;
  if (!userId) {
    throw new Error("e2e user context is required");
  }

  await withAuthPool(async (pool) => {
    await pool.query(`UPDATE "user" SET "activeProjectId" = $1 WHERE id = $2`, [
      projectId,
      userId,
    ]);
  });
}

/** Remove the e2e user's membership from an organization. */
export async function removeE2EUserMembership(orgId: string) {
  const userId = readRunContext()?.userId;
  if (!userId) {
    throw new Error("e2e user context is required");
  }

  await withAuthPool(async (pool) => {
    await pool.query(
      `DELETE FROM "member" WHERE "organizationId" = $1 AND "userId" = $2`,
      [orgId, userId]
    );
  });
}

/** Temporarily set the e2e user's organization role for RBAC browser checks. */
export async function setE2EUserOrganizationRole(
  orgId: string,
  role: "owner" | "admin" | "member"
) {
  const userId = readRunContext()?.userId;
  if (!userId) {
    throw new Error("e2e user context is required");
  }

  await withAuthPool(async (pool) => {
    await pool.query(
      `UPDATE "member" SET "role" = $1 WHERE "organizationId" = $2 AND "userId" = $3`,
      [role, orgId, userId]
    );
  });
}

/** Create an isolated auth org/project pair and sync it to the Go API. */
export async function createIsolatedOrgProject(
  api: ApiHelper,
  namePrefix: string
): Promise<IsolatedOrgProject> {
  loadE2EEnv();
  const authDbUrl = process.env.AUTH_DATABASE_URL;
  const userId = readRunContext()?.userId;
  if (!(authDbUrl && userId)) {
    throw new Error("AUTH_DATABASE_URL and e2e user context are required");
  }

  const orgId = crypto.randomUUID();
  const projectId = crypto.randomUUID();
  const projectName = `${namePrefix}-${projectId.slice(0, 8)}`;
  const orgSlug = `e2e-org-${orgId.slice(0, 8)}`;
  const projectSlug = `e2e-project-${projectId.slice(0, 8)}`;
  const pool = new pg.Pool({ connectionString: authDbUrl });

  try {
    await pool.query(
      `INSERT INTO "organization" ("id", "name", "slug", "createdAt")
       VALUES ($1, $2, $3, NOW())`,
      [orgId, `${namePrefix} Organization`, orgSlug]
    );
    await pool.query(
      `INSERT INTO "member" ("id", "organizationId", "userId", "role", "createdAt")
       VALUES ($1, $2, $3, 'owner', NOW())`,
      [crypto.randomUUID(), orgId, userId]
    );
    await pool.query(
      `INSERT INTO project (id, organization_id, name, slug, description, created_by)
       VALUES ($1, $2, $3, $4, '', $5)`,
      [projectId, orgId, projectName, projectSlug, userId]
    );
  } finally {
    await pool.end();
  }

  await api.createProject({ id: projectId, org_id: orgId, name: projectName });
  await ensureUnlimitedE2EPlan(orgId);
  return { orgId, projectId, projectName };
}

/** Remove an isolated auth org/project pair and its Go API project. */
export async function cleanupIsolatedOrgProject(
  api: ApiHelper,
  isolated: IsolatedOrgProject | null
) {
  if (!isolated) {
    return;
  }

  loadE2EEnv();
  await api.deleteProject(isolated.projectId).catch(() => undefined);

  const authDbUrl = process.env.AUTH_DATABASE_URL;
  if (!authDbUrl) {
    return;
  }

  const pool = new pg.Pool({ connectionString: authDbUrl });
  try {
    await pool.query("DELETE FROM project WHERE id = $1", [isolated.projectId]);
    await pool.query(`DELETE FROM "member" WHERE "organizationId" = $1`, [
      isolated.orgId,
    ]);
    await pool.query(`DELETE FROM "organization" WHERE "id" = $1`, [
      isolated.orgId,
    ]);
  } finally {
    await pool.end();
  }
}
