import {
  type QueryClient,
  queryOptions,
  useMutation,
  useQueryClient,
} from "@tanstack/react-query";
import type { Project } from "@/hooks/api/types";
import { queryKeys } from "@/hooks/query-keys";
import { DEFAULT_GC_TIME, DEFAULT_STALE_TIME } from "@/hooks/utils";
import { getPostHog } from "@/lib/analytics";
import {
  createProjectServerFn,
  listProjectsServerFn,
  setActiveProjectServerFn,
} from "@/lib/project-handler";

export const projectsQueryOptions = (organizationId: string) =>
  queryOptions({
    queryKey: queryKeys.projects.list(organizationId).queryKey,
    queryFn: () => listProjectsServerFn({ data: { organizationId } }),
    staleTime: DEFAULT_STALE_TIME,
    gcTime: DEFAULT_GC_TIME,
    enabled: !!organizationId,
  });

export const useCreateProject = () => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationKey: ["projects", "create"],
    mutationFn: (data: {
      organizationId: string;
      name: string;
      description?: string;
    }) => createProjectServerFn({ data }),
    onSuccess: (data) => {
      getPostHog()?.capture("project_created", { project_id: data?.id });
    },
    onError: (err) => {
      getPostHog()?.capture("mutation_error", {
        action: "project_created",
        error_message: err instanceof Error ? err.message : "Unknown error",
      });
    },
    onSettled: () => {
      queryClient.invalidateQueries({
        queryKey: queryKeys.projects._def,
      });
    },
  });
};

const invalidateProjectScopedQueries = (queryClient: QueryClient) => {
  queryClient.invalidateQueries({ queryKey: queryKeys.auth._def });
  queryClient.invalidateQueries({ queryKey: queryKeys.jobs._def });
  queryClient.invalidateQueries({ queryKey: queryKeys.runs._def });
  queryClient.invalidateQueries({ queryKey: queryKeys.workflows._def });
  queryClient.invalidateQueries({ queryKey: queryKeys.schedules._def });
  queryClient.invalidateQueries({ queryKey: queryKeys.webhooks._def });
  queryClient.invalidateQueries({ queryKey: queryKeys.events._def });
  queryClient.invalidateQueries({ queryKey: queryKeys.dlq._def });
};

export const useCreateAndActivateProject = () => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationKey: ["projects", "createAndActivate"],
    mutationFn: async (data: {
      organizationId: string;
      name: string;
      description?: string;
    }) => {
      const project = await createProjectServerFn({ data });
      await setActiveProjectServerFn({ data: { projectId: project.id } });
      return project;
    },
    onSuccess: (project, variables) => {
      getPostHog()?.capture("project_created", { project_id: project.id });
      getPostHog()?.capture("project_switched", {
        project_id: project.id,
      });
      queryClient.setQueryData<Project[]>(
        queryKeys.projects.list(variables.organizationId).queryKey,
        (projects) => {
          if (!projects) {
            return [project];
          }
          if (projects.some((existing) => existing.id === project.id)) {
            return projects;
          }
          return [...projects, project];
        }
      );
    },
    onError: (err) => {
      getPostHog()?.capture("mutation_error", {
        action: "project_created",
        error_message: err instanceof Error ? err.message : "Unknown error",
      });
    },
    onSettled: (_data, _error, variables) => {
      queryClient.invalidateQueries({
        queryKey: queryKeys.projects.list(variables.organizationId).queryKey,
      });
      invalidateProjectScopedQueries(queryClient);
    },
  });
};

export const useSetActiveProject = () => {
  const queryClient = useQueryClient();

  return useMutation({
    mutationKey: ["projects", "setActive"],
    mutationFn: (data: { projectId: string }) =>
      setActiveProjectServerFn({ data }),
    onSuccess: (_data, variables) => {
      getPostHog()?.capture("project_switched", {
        project_id: variables.projectId,
      });
      invalidateProjectScopedQueries(queryClient);
    },
    onError: (err, variables) => {
      getPostHog()?.capture("mutation_error", {
        action: "project_switched",
        error_message: err instanceof Error ? err.message : "Unknown error",
        project_id: variables.projectId,
      });
    },
  });
};
