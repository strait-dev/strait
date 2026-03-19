import { createServerFn } from "@tanstack/react-start";
import { getRequestHeaders } from "@tanstack/react-start/server";
import z from "zod/v4";
import type { Project } from "@/hooks/api/types";
import { apiRequest } from "@/lib/api-client.server";
import { auth, authPool } from "@/lib/auth.server";
import { authMiddleware } from "@/middlewares/auth";

/**
 * Ensures the project table exists in the auth database.
 * Called lazily on first project operation.
 */
let tableEnsured = false;
async function ensureProjectTable() {
  if (tableEnsured) {
    return;
  }
  await authPool.query(`
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
    await ensureProjectTable();

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
    try {
      await apiRequest("/v1/projects", {
        method: "POST",
        body: {
          id: project.id,
          org_id: data.organizationId,
          name: data.name,
        },
      });
    } catch (err) {
      console.error("Failed to sync project to service:", err);
    }

    return project;
  });

/** List projects for an organization. */
export const listProjectsServerFn = createServerFn({ method: "GET" })
  .inputValidator((data: { organizationId: string }) =>
    z.object({ organizationId: z.string().min(1) }).parse(data)
  )
  .middleware([authMiddleware])
  .handler(async ({ data }) => {
    await ensureProjectTable();

    const result = await authPool.query<Project>(
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
  .handler(async ({ data }) => {
    await ensureProjectTable();

    const result = await authPool.query<Project>(
      `SELECT id, organization_id, name, slug, description, created_by, created_at::text, updated_at::text
       FROM project
       WHERE id = $1`,
      [data.id]
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
    await ensureProjectTable();

    const result = await authPool.query(
      "DELETE FROM project WHERE id = $1 AND created_by = $2 RETURNING id",
      [data.id, context.user.id]
    );

    if (result.rowCount === 0) {
      throw new Error("Project not found or permission denied");
    }

    // Sync deletion to Go service (best-effort).
    try {
      await apiRequest(`/v1/projects/${data.id}`, { method: "DELETE" });
    } catch (err) {
      console.error("Failed to sync project deletion to service:", err);
    }

    return { success: true };
  });

/** Set the active project for the current user. */
export const setActiveProjectServerFn = createServerFn({ method: "POST" })
  .inputValidator((data: { projectId: string }) =>
    z.object({ projectId: z.string().min(1) }).parse(data)
  )
  .middleware([authMiddleware])
  .handler(async ({ data }) => {
    const headers = getRequestHeaders();
    await auth.api.updateUser({
      body: { activeProjectId: data.projectId },
      headers,
    });
    return { success: true };
  });
