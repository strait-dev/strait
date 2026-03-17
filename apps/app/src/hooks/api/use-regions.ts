import {
  queryOptions,
  useMutation,
  useQueryClient,
} from "@tanstack/react-query";

import { DEFAULT_GC_TIME, DEFAULT_STALE_TIME } from "@/hooks/utils";
import {
  fetchProjectSettings,
  fetchRegions,
  updateProjectSettingsFn,
} from "@/lib/api";

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
