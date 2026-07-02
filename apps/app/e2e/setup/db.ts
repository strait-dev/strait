import crypto from "node:crypto";
import pg from "pg";

const E2E_USER_NAME = "E2E Test User";

export async function ensureUserExists(
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

    await new Promise((r) => setTimeout(r, 2000));

    userRow = await pool.query(
      `SELECT "id", "defaultOrganizationId", "activeProjectId" FROM "user" WHERE "email" = $1`,
      [email]
    );
  }

  if (userRow.rows.length === 0) {
    throw new Error(
      `Failed to create test user ${email}. Check that the app is running at ${baseURL}`
    );
  }

  await pool.query(`UPDATE "user" SET "emailVerified" = true WHERE "id" = $1`, [
    userRow.rows[0].id,
  ]);

  return userRow.rows[0];
}

export async function ensureOrgExists(
  pool: pg.Pool,
  userId: string,
  existingOrgId: string | null
) {
  if (existingOrgId) {
    await pool.query(
      `INSERT INTO "member" ("id", "organizationId", "userId", "role", "createdAt")
       VALUES ($1, $2, $3, 'owner', NOW())
       ON CONFLICT DO NOTHING`,
      [crypto.randomUUID(), existingOrgId, userId]
    );
    await pool.query(
      `UPDATE "member"
       SET "role" = 'owner'
       WHERE "organizationId" = $1 AND "userId" = $2`,
      [existingOrgId, userId]
    );
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

export async function ensureProjectExists(
  pool: pg.Pool,
  userId: string,
  orgId: string,
  existingProjectId: string | null,
  apiURL: string,
  internalSecret: string
) {
  let projectId = existingProjectId;

  if (projectId) {
    await pool.query(
      "UPDATE project SET name = $1 WHERE id = $2 AND name = $3",
      ["Default project", projectId, "Default Project"]
    );
  } else {
    projectId = crypto.randomUUID();
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
      [projectId, orgId, "Default project", projectSlug, userId]
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
    internalSecret
  );
  if (!synced) {
    console.warn(
      `Project not synced to Go API. Data-seeded tests will fail. Ensure backend is running at ${apiURL}`
    );
  }

  return projectId;
}

export async function ensureLimitedMemberExists(
  pool: pg.Pool,
  email: string,
  password: string,
  baseURL: string,
  orgId: string,
  projectId: string
) {
  const user = await ensureUserExists(pool, email, password, baseURL);

  const existingMember = await pool.query(
    `SELECT "id" FROM "member" WHERE "organizationId" = $1 AND "userId" = $2 LIMIT 1`,
    [orgId, user.id]
  );
  if (existingMember.rows.length === 0) {
    await pool.query(
      `INSERT INTO "member" ("id", "organizationId", "userId", "role", "createdAt")
       VALUES ($1, $2, $3, 'member', NOW())`,
      [crypto.randomUUID(), orgId, user.id]
    );
  }
  await pool.query(
    `UPDATE "member"
     SET "role" = 'member'
     WHERE "organizationId" = $1 AND "userId" = $2`,
    [orgId, user.id]
  );
  await pool.query(
    `UPDATE "user"
     SET "defaultOrganizationId" = $1, "activeProjectId" = $2, "emailVerified" = true
     WHERE "id" = $3`,
    [orgId, projectId, user.id]
  );

  return user;
}

export async function ensureProjectRbacExists(
  pool: pg.Pool,
  projectId: string,
  ownerUserId: string,
  limitedUserId: string
) {
  const ownerRoleId = `e2e-owner-${projectId}`;
  const limitedRoleId = `e2e-limited-${projectId}`;

  await pool.query(
    `
      INSERT INTO project_roles (
        id, project_id, name, description, permissions, is_system
      )
      VALUES ($1, $2, 'E2E Owner', 'Full local dogfood access', $3, true)
      ON CONFLICT (project_id, name) DO UPDATE SET
        permissions = EXCLUDED.permissions,
        updated_at = NOW()
    `,
    [ownerRoleId, projectId, ["*"]]
  );

  await pool.query(
    `
      INSERT INTO project_roles (
        id, project_id, name, description, permissions, is_system
      )
      VALUES ($1, $2, 'E2E Read Only', 'Read-only local dogfood access', $3, true)
      ON CONFLICT (project_id, name) DO UPDATE SET
        permissions = EXCLUDED.permissions,
        updated_at = NOW()
    `,
    [
      limitedRoleId,
      projectId,
      [
        "jobs:read",
        "runs:read",
        "workflows:read",
        "webhooks:read",
        "stats:read",
        "projects:read",
        "dlq:read",
        "outbox:read",
      ],
    ]
  );

  await pool.query(
    `
      INSERT INTO project_member_roles (
        id, project_id, user_id, role_id, granted_by
      )
      VALUES ($1, $2, $3, $4, $3)
      ON CONFLICT (project_id, user_id) DO UPDATE SET
        role_id = EXCLUDED.role_id,
        granted_by = EXCLUDED.granted_by
    `,
    [crypto.randomUUID(), projectId, ownerUserId, ownerRoleId]
  );

  await pool.query(
    `
      INSERT INTO project_member_roles (
        id, project_id, user_id, role_id, granted_by
      )
      VALUES ($1, $2, $3, $4, $5)
      ON CONFLICT (project_id, user_id) DO UPDATE SET
        role_id = EXCLUDED.role_id,
        granted_by = EXCLUDED.granted_by
    `,
    [crypto.randomUUID(), projectId, limitedUserId, limitedRoleId, ownerUserId]
  );
}

async function syncProjectToApi(
  projectId: string,
  orgId: string,
  apiURL: string,
  internalSecret: string
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
        name: "Default project",
      }),
    });
    if (res.ok || res.status === 409) {
      return true;
    }

    if (res.status === 402) {
      return await projectExists(projectId, apiURL, internalSecret);
    }

    return false;
  } catch {
    return false;
  }
}

async function projectExists(
  projectId: string,
  apiURL: string,
  internalSecret: string
) {
  try {
    const res = await fetch(`${apiURL}/v1/projects/${projectId}`, {
      headers: {
        "X-Internal-Secret": internalSecret,
        "X-Project-Id": projectId,
      },
    });
    return res.ok;
  } catch {
    return false;
  }
}

export async function cleanupTestUser(authDbUrl: string, email: string) {
  const pool = new pg.Pool({ connectionString: authDbUrl });

  try {
    const userResult = await pool.query(
      `SELECT "id" FROM "user" WHERE "email" = $1`,
      [email]
    );

    if (userResult.rows.length === 0) {
      return;
    }

    const userId = userResult.rows[0].id;

    // Delete projects
    try {
      await pool.query("DELETE FROM project WHERE created_by = $1", [userId]);
    } catch {
      // project table may not exist
    }

    // Delete in dependency order
    await pool.query(`DELETE FROM "session" WHERE "userId" = $1`, [userId]);
    await pool.query(`DELETE FROM "account" WHERE "userId" = $1`, [userId]);

    const orgs = await pool.query(
      `SELECT "organizationId" FROM "member" WHERE "userId" = $1`,
      [userId]
    );
    await pool.query(`DELETE FROM "member" WHERE "userId" = $1`, [userId]);

    for (const row of orgs.rows) {
      const memberCount = await pool.query(
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
