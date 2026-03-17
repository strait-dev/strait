import { queryOptions, useMutation } from "@tanstack/react-query";
import type { Region, ProjectSettings } from "@/hooks/api/types";
import { DEFAULT_GC_TIME, DEFAULT_STALE_TIME } from "@/hooks/utils";

// --- Region list ---

const MOCK_REGIONS: Region[] = [
  {
    code: "iad",
    label: "Ashburn, Virginia (US)",
    city: "Ashburn",
    country: "US",
    continent: "North America",
    availability: { free: true, starter: true, professional: true, enterprise: true },
  },
  {
    code: "lhr",
    label: "London, United Kingdom",
    city: "London",
    country: "GB",
    continent: "Europe",
    availability: { free: false, starter: true, professional: true, enterprise: true },
  },
  {
    code: "fra",
    label: "Frankfurt, Germany",
    city: "Frankfurt",
    country: "DE",
    continent: "Europe",
    availability: { free: false, starter: true, professional: true, enterprise: true },
  },
  {
    code: "nrt",
    label: "Tokyo, Japan",
    city: "Tokyo",
    country: "JP",
    continent: "Asia",
    availability: { free: false, starter: true, professional: true, enterprise: true },
  },
  {
    code: "syd",
    label: "Sydney, Australia",
    city: "Sydney",
    country: "AU",
    continent: "Oceania",
    availability: { free: false, starter: true, professional: true, enterprise: true },
  },
  {
    code: "lax",
    label: "Los Angeles, California (US)",
    city: "Los Angeles",
    country: "US",
    continent: "North America",
    availability: { free: false, starter: true, professional: true, enterprise: true },
  },
  {
    code: "hkg",
    label: "Hong Kong",
    city: "Hong Kong",
    country: "HK",
    continent: "Asia",
    availability: { free: false, starter: false, professional: true, enterprise: true },
  },
  {
    code: "gru",
    label: "São Paulo, Brazil",
    city: "São Paulo",
    country: "BR",
    continent: "South America",
    availability: { free: false, starter: false, professional: true, enterprise: true },
  },
];

async function listRegions(): Promise<{ regions: Region[] }> {
  // TODO: Replace with real API call: GET /v1/regions
  await Promise.resolve();
  return { regions: MOCK_REGIONS };
}

export const regionsQueryOptions = () =>
  queryOptions({
    queryKey: ["regions"],
    queryFn: () => listRegions(),
    staleTime: DEFAULT_STALE_TIME * 10, // regions change rarely
    gcTime: DEFAULT_GC_TIME * 10,
  });

// --- Project settings ---

async function getProjectSettings(
  projectId: string
): Promise<ProjectSettings> {
  // TODO: Replace with real API call: GET /v1/projects/:id/settings
  await Promise.resolve();
  return {
    project_id: projectId,
    default_region: "",
    plan_tier: "free",
  };
}

async function updateProjectSettings(data: {
  projectId: string;
  default_region: string;
}): Promise<ProjectSettings> {
  // TODO: Replace with real API call: PUT /v1/projects/:id/settings
  await Promise.resolve();
  return {
    project_id: data.projectId,
    default_region: data.default_region,
    plan_tier: "free",
  };
}

export const projectSettingsQueryOptions = (projectId: string) =>
  queryOptions({
    queryKey: ["project-settings", projectId],
    queryFn: () => getProjectSettings(projectId),
    staleTime: DEFAULT_STALE_TIME,
    gcTime: DEFAULT_GC_TIME,
  });

export const useUpdateProjectSettings = () =>
  useMutation({
    mutationKey: ["project-settings", "update"],
    mutationFn: (data: { projectId: string; default_region: string }) =>
      updateProjectSettings(data),
  });
