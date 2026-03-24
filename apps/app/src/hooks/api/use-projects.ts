import {
  queryOptions,
  useMutation,
  useQueryClient,
} from "@tanstack/react-query";
import { queryKeys } from "@/hooks/query-keys";
import { DEFAULT_GC_TIME, DEFAULT_STALE_TIME } from "@/hooks/utils";
import {
  createProjectServerFn,
  deleteProjectServerFn,
  getProjectServerFn,
  listProjectsServerFn,
  setActiveProjectServerFn,
} from "@/lib/project-handler";

// ---------------------------------------------------------------------------
// Query options
// ---------------------------------------------------------------------------

export const projectsQueryOptions = (organizationId: string) =>
  queryOptions({
    queryKey: queryKeys.projects.list(organizationId).queryKey,
    queryFn: () => listProjectsServerFn({ data: { organizationId } }),
    staleTime: DEFAULT_STALE_TIME,
    gcTime: DEFAULT_GC_TIME,
    enabled: !!organizationId,
  });

export const projectQueryOptions = (id: string) =>
  queryOptions({
    queryKey: queryKeys.projects.detail(id).queryKey,
    queryFn: () => getProjectServerFn({ data: { id } }),
    staleTime: DEFAULT_STALE_TIME,
    gcTime: DEFAULT_GC_TIME,
    enabled: !!id,
  });

// ---------------------------------------------------------------------------
// Mutations
// ---------------------------------------------------------------------------

export const useCreateProject = () => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationKey: ["projects", "create"],
    mutationFn: (data: {
      organizationId: string;
      name: string;
      description?: string;
    }) => createProjectServerFn({ data }),
    onSettled: () => {
      queryClient.invalidateQueries({
        queryKey: queryKeys.projects._def,
      });
    },
  });
};

export const useDeleteProject = () => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationKey: ["projects", "delete"],
    mutationFn: (data: { id: string }) => deleteProjectServerFn({ data }),
    onSettled: () => {
      queryClient.invalidateQueries({
        queryKey: queryKeys.projects._def,
      });
    },
  });
};

export const useSetActiveProject = () => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationKey: ["projects", "setActive"],
    mutationFn: (data: { projectId: string }) =>
      setActiveProjectServerFn({ data }),
    onSuccess: () => {
      // Invalidate all project-scoped data queries
      queryClient.invalidateQueries({ queryKey: queryKeys.jobs._def });
      queryClient.invalidateQueries({ queryKey: queryKeys.runs._def });
      queryClient.invalidateQueries({ queryKey: queryKeys.workflows._def });
      queryClient.invalidateQueries({ queryKey: queryKeys.schedules._def });
      queryClient.invalidateQueries({ queryKey: queryKeys.webhooks._def });
      queryClient.invalidateQueries({ queryKey: queryKeys.events._def });
      queryClient.invalidateQueries({ queryKey: queryKeys.dlq._def });
    },
  });
};
