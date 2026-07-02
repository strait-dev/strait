import { createServerFn } from "@tanstack/react-start";
import z from "zod/v4";
import type { Project } from "@/hooks/api/types";
import { authMiddleware } from "@/middlewares/auth";

/**
 * Ensures the project table exists in the auth database.
 * Called lazily on first project operation.
 */
let tableEnsured = false;
export async function ensureProjectTable(pool: {
  query: (sql: string) => Promise<unknown>;
}) {
  if (tableEnsured) {
    return;
  }
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
  tableEnsured = true;
}

/** Create a new project under an organization. */
export const createProjectServerFn = createServerFn({ method: "POST" })
  .inputValidator(
    (data: { organizationId: string; name: string; description?: string }) =>
      z
        .object({
          organizationId: z.string().min(1),
          name: z.string().min(2),
          description: z.string().optional(),
        })
        .parse(data)
  )
  .middleware([authMiddleware])
  .handler(async ({ context, data }) => {
    const [
      { getAuthPool },
      { requireOrgAccess },
      { apiEffect, runWithFallback },
    ] = await Promise.all([
      import("@/lib/auth.server"),
      import("@/middlewares/require-access"),
      import("@/lib/effect-api.server"),
    ]);
    await requireOrgAccess(context.user.id, data.organizationId);
    const authPool = getAuthPool();
    await ensureProjectTable(authPool);

    const slug = data.name
      .toLowerCase()
      .replace(/\s+/g, "-")
      .replace(/[^a-z0-9-]/g, "");

    const result = await authPool.query<Project>(
      `INSERT INTO project (organization_id, name, slug, description, created_by)
       VALUES ($1, $2, $3, $4, $5)
       RETURNING id, organization_id, name, slug, description, created_by, created_at::text, updated_at::text`,
      [
        data.organizationId,
        data.name,
        slug,
        data.description ?? "",
        context.user.id,
      ]
    );

    const project = result.rows[0];

    // Sync to Go service (best-effort).
    await runWithFallback(
      apiEffect("/v1/projects", {
        method: "POST",
        body: {
          id: project.id,
          org_id: data.organizationId,
          name: data.name,
        },
      }),
      undefined
    );

    return project;
  });

/** List projects for an organization. */
export const listProjectsServerFn = createServerFn({ method: "GET" })
  .inputValidator((data: { organizationId: string }) =>
    z.object({ organizationId: z.string().min(1) }).parse(data)
  )
  .middleware([authMiddleware])
  .handler(async ({ context, data }) => {
    const [{ getAuthPool }, { requireOrgAccess }] = await Promise.all([
      import("@/lib/auth.server"),
      import("@/middlewares/require-access"),
    ]);
    const authPool = getAuthPool();
    await Promise.all([
      requireOrgAccess(context.user.id, data.organizationId),
      ensureProjectTable(authPool),
    ]);

    const result = await authPool.query<Project>(
      `SELECT id, organization_id, name, slug, description, created_by, created_at::text, updated_at::text
       FROM project
       WHERE organization_id = $1
       ORDER BY created_at ASC`,
      [data.organizationId]
    );

    return result.rows;
  });

/** Set the active project for the current user. */
export const setActiveProjectServerFn = createServerFn({ method: "POST" })
  .inputValidator((data: { projectId: string }) =>
    z.object({ projectId: z.string().min(1) }).parse(data)
  )
  .middleware([authMiddleware])
  .handler(async ({ context, data }) => {
    const [{ getAuthPool }, { requireProjectAccess }] = await Promise.all([
      import("@/lib/auth.server"),
      import("@/middlewares/require-access"),
    ]);
    const activeOrgId = (context as Record<string, unknown>)
      .activeOrganizationId as string | undefined;
    await requireProjectAccess(context.user.id, data.projectId, activeOrgId);

    await getAuthPool().query(
      `UPDATE "user" SET "activeProjectId" = $1 WHERE id = $2`,
      [data.projectId, context.user.id]
    );
    return { success: true };
  });
