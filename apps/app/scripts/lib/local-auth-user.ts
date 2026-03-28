import crypto from "node:crypto";
import pg from "pg";

export const DEFAULT_LOCAL_DEV_USER_EMAIL = "dev@local.strait";
export const DEFAULT_LOCAL_DEV_USER_PASSWORD = "devpassword123";
export const DEFAULT_LOCAL_DEV_USER_NAME = "Local Dev User";

export type SeededAuthUser = {
  id: string;
  defaultOrganizationId: string | null;
  activeProjectId: string | null;
};

type EnsurePasswordUserOptions = {
  email: string;
  password: string;
  baseURL: string;
  name?: string;
  waitForMs?: number;
  markEmailVerified?: boolean;
};

type EnsureOrganizationOptions = {
  userId: string;
  existingOrgId: string | null;
  workspaceName?: string;
};

type EnsureProjectOptions = {
  userId: string;
  orgId: string;
  existingProjectId: string | null;
  apiURL: string;
  internalSecret: string;
  projectName?: string;
};

type EnsureLocalDevUserOptions = {
  authDbUrl: string;
  baseURL: string;
  apiURL: string;
  internalSecret: string;
  email?: string;
  password?: string;
  name?: string;
};

async function sleep(ms: number) {
  await new Promise((resolve) => setTimeout(resolve, ms));
}

async function ensureProjectTable(pool: pg.Pool) {
  await pool.query(`
    CREATE TABLE IF NOT EXISTS project (
      id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
      organization_id TEXT NOT NULL REFERENCES organization(id) ON DELETE CASCADE,
      name TEXT NOT NULL,
      slug TEXT NOT NULL,
      description TEXT DEFAULT '',
      created_by TEXT NOT NULL REFERENCES "user"(id),
      created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
      updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
      UNIQUE(organization_id, slug)
    )
  `);
}

export async function ensurePasswordUserExists(
  pool: pg.Pool,
  options: EnsurePasswordUserOptions
) {
  const {
    email,
    password,
    baseURL,
    name = DEFAULT_LOCAL_DEV_USER_NAME,
    waitForMs = 2_000,
    markEmailVerified = true,
  } = options;

  let userRow = await pool.query<SeededAuthUser>(
    `SELECT "id", "defaultOrganizationId", "activeProjectId" FROM "user" WHERE "email" = $1`,
    [email]
  );

  if (userRow.rows.length === 0) {
    const signupRes = await fetch(`${baseURL}/api/auth/sign-up/email`, {
      method: "POST",
      headers: { "Content-Type": "application/json", Origin: baseURL },
      body: JSON.stringify({ name, email, password }),
    });

    if (!signupRes.ok) {
      throw new Error(
        `Failed to create local auth user ${email}: ${signupRes.status} ${await signupRes.text()}`
      );
    }

    await sleep(waitForMs);

    userRow = await pool.query<SeededAuthUser>(
      `SELECT "id", "defaultOrganizationId", "activeProjectId" FROM "user" WHERE "email" = $1`,
      [email]
    );
  }

  if (userRow.rows.length === 0) {
    throw new Error(
      `Failed to find local auth user ${email} after sign-up completed`
    );
  }

  if (markEmailVerified) {
    await pool.query(
      `UPDATE "user" SET "emailVerified" = true WHERE "id" = $1`,
      [userRow.rows[0].id]
    );
  }

  return userRow.rows[0];
}

export async function ensureOrganizationExists(
  pool: pg.Pool,
  options: EnsureOrganizationOptions
) {
  const {
    userId,
    existingOrgId,
    workspaceName = `${DEFAULT_LOCAL_DEV_USER_NAME}'s Workspace`,
  } = options;

  if (existingOrgId) {
    return existingOrgId;
  }

  const orgId = crypto.randomUUID();
  const orgSlug = `ws-${crypto.randomUUID().slice(0, 12)}`;

  await pool.query(
    `INSERT INTO "organization" ("id", "name", "slug", "createdAt")
     VALUES ($1, $2, $3, NOW()) ON CONFLICT DO NOTHING`,
    [orgId, workspaceName, orgSlug]
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

async function syncProjectToApi(
  projectId: string,
  orgId: string,
  apiURL: string,
  internalSecret: string,
  projectName: string
) {
  if (!internalSecret) {
    return false;
  }

  try {
    const res = await fetch(`${apiURL}/v1/projects`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        "X-Internal-Secret": internalSecret,
      },
      body: JSON.stringify({
        id: projectId,
        org_id: orgId,
        name: projectName,
      }),
    });
    return res.ok || res.status === 409;
  } catch {
    return false;
  }
}

export async function ensureProjectExists(
  pool: pg.Pool,
  options: EnsureProjectOptions
) {
  const {
    userId,
    orgId,
    existingProjectId,
    apiURL,
    internalSecret,
    projectName = "Default Project",
  } = options;

  let projectId = existingProjectId;

  if (!projectId) {
    projectId = crypto.randomUUID();
    const projectSlug = `project-${projectId.slice(0, 8)}`;

    await ensureProjectTable(pool);

    await pool.query(
      `INSERT INTO project (id, organization_id, name, slug, created_by)
       VALUES ($1, $2, $3, $4, $5) ON CONFLICT DO NOTHING`,
      [projectId, orgId, projectName, projectSlug, userId]
    );

    await pool.query(
      `UPDATE "user" SET "activeProjectId" = $1 WHERE "id" = $2`,
      [projectId, userId]
    );
  }

  const synced = await syncProjectToApi(
    projectId,
    orgId,
    apiURL,
    internalSecret,
    projectName
  );
  if (!synced) {
    console.warn(
      `Project ${projectId} not synced to Go API at ${apiURL}. Start the Go service to use backend-backed app features.`
    );
  }

  return projectId;
}

export async function ensureLocalDevUser(options: EnsureLocalDevUserOptions) {
  const {
    authDbUrl,
    baseURL,
    apiURL,
    internalSecret,
    email = DEFAULT_LOCAL_DEV_USER_EMAIL,
    password = DEFAULT_LOCAL_DEV_USER_PASSWORD,
    name = DEFAULT_LOCAL_DEV_USER_NAME,
  } = options;

  const pool = new pg.Pool({ connectionString: authDbUrl });

  try {
    const user = await ensurePasswordUserExists(pool, {
      email,
      password,
      baseURL,
      name,
    });
    const orgId = await ensureOrganizationExists(pool, {
      userId: user.id,
      existingOrgId: user.defaultOrganizationId,
      workspaceName: `${name}'s Workspace`,
    });
    const projectId = await ensureProjectExists(pool, {
      userId: user.id,
      orgId,
      existingProjectId: user.activeProjectId,
      apiURL,
      internalSecret,
    });

    return {
      email,
      password,
      name,
      userId: user.id,
      orgId,
      projectId,
    };
  } finally {
    await pool.end();
  }
}

export async function cleanupAuthUser(authDbUrl: string, email: string) {
  const pool = new pg.Pool({ connectionString: authDbUrl });

  try {
    const userResult = await pool.query<{ id: string }>(
      `SELECT "id" FROM "user" WHERE "email" = $1`,
      [email]
    );

    if (userResult.rows.length === 0) {
      return;
    }

    const userId = userResult.rows[0].id;

    try {
      await pool.query("DELETE FROM project WHERE created_by = $1", [userId]);
    } catch {
      // project table may not exist yet
    }

    await pool.query(`DELETE FROM "session" WHERE "userId" = $1`, [userId]);
    await pool.query(`DELETE FROM "account" WHERE "userId" = $1`, [userId]);

    const orgs = await pool.query<{ organizationId: string }>(
      `SELECT "organizationId" FROM "member" WHERE "userId" = $1`,
      [userId]
    );
    await pool.query(`DELETE FROM "member" WHERE "userId" = $1`, [userId]);

    for (const row of orgs.rows) {
      const memberCount = await pool.query<{ count: string }>(
        `SELECT COUNT(*) FROM "member" WHERE "organizationId" = $1`,
        [row.organizationId]
      );
      if (Number.parseInt(memberCount.rows[0].count, 10) === 0) {
        await pool.query(
          `DELETE FROM "invitation" WHERE "organizationId" = $1`,
          [row.organizationId]
        );
        await pool.query(`DELETE FROM "organization" WHERE "id" = $1`, [
          row.organizationId,
        ]);
      }
    }

    await pool.query(`DELETE FROM "user" WHERE "id" = $1`, [userId]);
  } finally {
    await pool.end();
  }
}
