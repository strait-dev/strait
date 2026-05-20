import {
  queryOptions,
  useMutation,
  useQueryClient,
} from "@tanstack/react-query";
import { createServerFn } from "@tanstack/react-start";
import z from "zod/v4";
import type { ProjectSettings, Region } from "@/hooks/api/types";
import { DEFAULT_GC_TIME, DEFAULT_STALE_TIME } from "@/hooks/utils";
import { getPostHog } from "@/lib/analytics";
import { apiPath } from "@/lib/api-client.server";
import { apiEffect, runWithSentryReport } from "@/lib/effect-api.server";
import { authMiddleware } from "@/middlewares/auth";
import {
  requireProjectAccess,
  requireProjectAdmin,
} from "@/middlewares/require-access";

export const fetchRegions = createServerFn({ method: "GET" })
  .middleware([authMiddleware])
  .handler(
    async () =>
      await runWithSentryReport(apiEffect<{ regions: Region[] }>("/v1/regions"))
  );

export const fetchProjectSettings = createServerFn({ method: "GET" })
  .inputValidator((data: { projectId: string }) =>
    z.object({ projectId: z.string().min(1) }).parse(data)
  )
  .middleware([authMiddleware])
  .handler(async ({ context, data }) => {
    const activeOrgId = (context as Record<string, unknown>)
      .activeOrganizationId as string | undefined;
    await requireProjectAccess(context.user.id, data.projectId, activeOrgId);

    return await runWithSentryReport(
      apiEffect<ProjectSettings>(
        apiPath`/v1/projects/${data.projectId}/settings`
      )
    );
  });

export const updateProjectSettingsFn = createServerFn({ method: "POST" })
  .inputValidator((data: { projectId: string; default_region: string }) =>
    z
      .object({
        projectId: z.string().min(1),
        default_region: z.string().min(1),
      })
      .parse(data)
  )
  .middleware([authMiddleware])
  .handler(async ({ context, data }) => {
    const activeOrgId = (context as Record<string, unknown>)
      .activeOrganizationId as string | undefined;
    await requireProjectAdmin(context.user.id, data.projectId, activeOrgId);

    return await runWithSentryReport(
      apiEffect<ProjectSettings>(
        apiPath`/v1/projects/${data.projectId}/settings`,
        {
          method: "PUT",
          body: { default_region: data.default_region },
        }
      )
    );
  });

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
    enabled: !!projectId,
    staleTime: DEFAULT_STALE_TIME,
    gcTime: DEFAULT_GC_TIME,
  });

export const useUpdateProjectSettings = () => {
  const queryClient = useQueryClient();
  return useMutation({
    mutationKey: ["project-settings", "update"],
    mutationFn: (data: { projectId: string; default_region: string }) =>
      updateProjectSettingsFn({ data }),
    onSuccess: (_data, variables) => {
      getPostHog()?.capture("project_settings_updated", {
        project_id: variables.projectId,
        setting_key: "default_region",
      });
    },
    onError: (err, variables) => {
      getPostHog()?.capture("mutation_error", {
        action: "project_settings_updated",
        error_message: err instanceof Error ? err.message : "Unknown error",
        project_id: variables.projectId,
      });
    },
    onSettled: (_data, _err, variables) => {
      queryClient.invalidateQueries({
        queryKey: ["project-settings", variables.projectId],
      });
    },
  });
};
