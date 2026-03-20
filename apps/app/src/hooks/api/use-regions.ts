import {
  queryOptions,
  useMutation,
  useQueryClient,
} from "@tanstack/react-query";
import { createServerFn } from "@tanstack/react-start";
import type {
  PaginatedResponse,
  ProjectSettings,
  Region,
} from "@/hooks/api/types";
import { DEFAULT_GC_TIME, DEFAULT_STALE_TIME } from "@/hooks/utils";
import { apiEffect, runWithSentryReport } from "@/lib/effect-api.server";
import { authMiddleware } from "@/middlewares/auth";
import { requireProjectAccess } from "@/middlewares/require-access";

// ---------------------------------------------------------------------------
// Server functions
// ---------------------------------------------------------------------------

export const fetchRegions = createServerFn({ method: "GET" })
  .middleware([authMiddleware])
  .handler(async () => {
    return await runWithSentryReport(
      apiEffect<PaginatedResponse<Region>>("/v1/regions")
    );
  });

export const fetchProjectSettings = createServerFn({ method: "GET" })
  .inputValidator((data: { projectId: string }) => data)
  .middleware([authMiddleware])
  .handler(async ({ context, data }) => {
    const activeOrgId = (context as Record<string, unknown>).activeOrganizationId as string | undefined;
    await requireProjectAccess(context.user.id, data.projectId, activeOrgId);

    return await runWithSentryReport(
      apiEffect<ProjectSettings>(`/v1/projects/${data.projectId}/settings`)
    );
  });

export const updateProjectSettingsFn = createServerFn({ method: "POST" })
  .inputValidator((data: { projectId: string; default_region: string }) => data)
  .middleware([authMiddleware])
  .handler(async ({ context, data }) => {
    const activeOrgId = (context as Record<string, unknown>).activeOrganizationId as string | undefined;
    await requireProjectAccess(context.user.id, data.projectId, activeOrgId);

    return await runWithSentryReport(
      apiEffect<ProjectSettings>(`/v1/projects/${data.projectId}/settings`, {
        method: "PUT",
        body: { default_region: data.default_region },
      })
    );
  });

// ---------------------------------------------------------------------------
// Query options
// ---------------------------------------------------------------------------

export const regionsQueryOptions = () =>
  queryOptions({
    queryKey: ["regions"],
    queryFn: () => fetchRegions(),
    staleTime: DEFAULT_STALE_TIME * 10,
    gcTime: DEFAULT_GC_TIME * 10,
  });

export const projectSettingsQueryOptions = (projectId: string) =>
  queryOptions({
    queryKey: ["project-settings", projectId],
    queryFn: () => fetchProjectSettings({ data: { projectId } }),
    staleTime: DEFAULT_STALE_TIME,
    gcTime: DEFAULT_GC_TIME,
  });

// ---------------------------------------------------------------------------
// Mutations
// ---------------------------------------------------------------------------

export const useUpdateProjectSettings = () => {
  const queryClient = useQueryClient();
  return useMutation({
    mutationKey: ["project-settings", "update"],
    mutationFn: (data: { projectId: string; default_region: string }) =>
      updateProjectSettingsFn({ data }),
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({
        queryKey: ["project-settings", variables.projectId],
      });
    },
  });
};
