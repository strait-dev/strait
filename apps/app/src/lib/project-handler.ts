import { createServerFn } from "@tanstack/react-start";
import { getRequestHeaders } from "@tanstack/react-start/server";
import z from "zod/v4";
import type { Project } from "@/hooks/api/types";
import { apiPath } from "@/lib/api-client.server";
import { getAuth, getAuthPool } from "@/lib/auth.server";
import {
  apiEffect,
  runWithFallback,
  runWithSentryReport,
} from "@/lib/effect-api.server";
import { authMiddleware } from "@/middlewares/auth";
import {
  requireOrgAccess,
  requireProjectAccess,
  requireProjectAdmin,
} from "@/middlewares/require-access";

/**
 * Ensures the project table exists in the auth database.
 * Called lazily on first project operation.
 */
let tableEnsured = false;
export async function ensureProjectTable() {
  if (tableEnsured) {
    return;
  }
  await getAuthPool().query(`
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
    await requireOrgAccess(context.user.id, data.organizationId);
    await ensureProjectTable();

    const slug = data.name
      .toLowerCase()
      .replace(/\s+/g, "-")
      .replace(/[^a-z0-9-]/g, "");

    const result = await getAuthPool().query<Project>(
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
    await requireOrgAccess(context.user.id, data.organizationId);
    await ensureProjectTable();

    const result = await getAuthPool().query<Project>(
      `SELECT id, organization_id, name, slug, description, created_by, created_at::text, updated_at::text
       FROM project
       WHERE organization_id = $1
       ORDER BY created_at ASC`,
      [data.organizationId]
    );

    return result.rows;
  });

/** Get a single project by ID. */
export const getProjectServerFn = createServerFn({ method: "GET" })
  .inputValidator((data: { id: string }) =>
    z.object({ id: z.string().min(1) }).parse(data)
  )
  .middleware([authMiddleware])
  .handler(async ({ context, data }) => {
    await ensureProjectTable();

    const activeOrgId = (context as Record<string, unknown>)
      .activeOrganizationId as string | undefined;

    if (!activeOrgId) {
      return null;
    }

    const result = await getAuthPool().query<Project>(
      `SELECT id, organization_id, name, slug, description, created_by, created_at::text, updated_at::text
       FROM project
       WHERE id = $1 AND organization_id = $2`,
      [data.id, activeOrgId]
    );
    return result.rows[0] ?? null;
  });

/** Delete a project by ID (must be owned by current user or org admin). */
export const deleteProjectServerFn = createServerFn({ method: "POST" })
  .inputValidator((data: { id: string }) =>
    z.object({ id: z.string().min(1) }).parse(data)
  )
  .middleware([authMiddleware])
  .handler(async ({ context, data }) => {
    const activeOrgId = (context as Record<string, unknown>)
      .activeOrganizationId as string | undefined;
    if (!activeOrgId) {
      throw new Error("Forbidden");
    }
    await ensureProjectTable();
    await requireProjectAdmin(context.user.id, data.id, activeOrgId);

    const projectResult = await getAuthPool().query<{ id: string }>(
      "SELECT id FROM project WHERE id = $1 AND organization_id = $2",
      [data.id, activeOrgId]
    );

    if (projectResult.rowCount === 0) {
      throw new Error("Project not found or permission denied");
    }

    await runWithSentryReport(
      apiEffect(apiPath`/v1/projects/${data.id}`, {
        method: "DELETE",
        projectId: data.id,
      })
    );

    const result = await getAuthPool().query(
      "DELETE FROM project WHERE id = $1 AND organization_id = $2 RETURNING id",
      [data.id, activeOrgId]
    );

    if (result.rowCount === 0) {
      throw new Error("Project not found or permission denied");
    }

    await getAuthPool().query(
      `UPDATE "user"
       SET "activeProjectId" = NULL
       WHERE "activeProjectId" = $1`,
      [data.id]
    );

    return { success: true };
  });

/** Set the active project for the current user. */
export const setActiveProjectServerFn = createServerFn({ method: "POST" })
  .inputValidator((data: { projectId: string }) =>
    z.object({ projectId: z.string().min(1) }).parse(data)
  )
  .middleware([authMiddleware])
  .handler(async ({ context, data }) => {
    const activeOrgId = (context as Record<string, unknown>)
      .activeOrganizationId as string | undefined;
    await requireProjectAccess(context.user.id, data.projectId, activeOrgId);

    const headers = getRequestHeaders();
    await (await getAuth()).api.updateUser({
      body: { activeProjectId: data.projectId },
      headers,
    });
    return { success: true };
  });
