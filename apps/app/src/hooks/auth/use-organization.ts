import {
  keepPreviousData,
  queryOptions,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { createServerFn } from "@tanstack/react-start";
import { getRequestHeaders } from "@tanstack/react-start/server";
import type z from "zod/v4";
import { queryKeys } from "@/hooks/query-keys";
import { DEFAULT_GC_TIME } from "@/hooks/utils";
import { getPostHog } from "@/lib/analytics";
import { getAuth } from "@/lib/auth.server";
import {
  deleteLastOrganizationWithTokenServerFn,
  deleteOrganizationWithTokenServerFn,
  requestOrganizationDeletionServerFn,
  resendOrganizationDeletionCodeServerFn,
  setActiveOrganizationAuth,
  verifyOrganizationDeletionServerFn,
} from "@/lib/organization-handler";
import type {
  DeleteLastOrganizationWithTokenSchema,
  DeleteOrganizationWithTokenSchema,
  RequestOrganizationDeletionSchema,
  ResendOrganizationDeletionCodeResponseSchema,
  ResendOrganizationDeletionCodeSchema,
  VerifyOrganizationDeletionResponseSchema,
  VerifyOrganizationDeletionSchema,
} from "@/lib/schema";

export type OrganizationData = {
  id: string;
  name: string;
  slug: string | null;
  logo: string | null;
  metadata: any;
  createdAt: Date;
  updatedAt: Date;
};

/** API response format for organizations list. */
export type OrganizationsApiResponse = {
  page: OrganizationData[];
  pageCount: number;
};

/** Legacy type kept for backwards compatibility - Convex doesn't use pagination. */
export type UseOrganizationsProps = {
  search?: {
    page?: number;
    perPage?: number;
    sort?: string;
    from?: string;
    to?: string;
  };
};

const toDate = (value: unknown) =>
  value instanceof Date ? value : new Date(String(value));

const mapOrganization = (org: {
  id: string;
  name: string;
  slug?: string | null;
  logo?: string | null;
  metadata?: unknown;
  createdAt?: unknown;
  updatedAt?: unknown;
}): OrganizationData => ({
  id: org.id,
  name: org.name,
  slug: org.slug ?? null,
  logo: org.logo ?? null,
  metadata: org.metadata ?? null,
  createdAt: toDate(org.createdAt),
  updatedAt: toDate(org.updatedAt ?? org.createdAt),
});

/** Parameters for updating an organization. */
interface UpdateOrganizationParams {
  activity?: string | null;
  address?: string | null;
  city?: string | null;
  companySize?: string | null;
  country?: string | null;
  createdAt?: Date;
  currency?: string | null;
  description?: string | null;
  email?: string | null;
  employeesSize?: string | null;
  fiscalType?: string | null;
  foundedAt?: Date | null;
  id?: string;
  language?: string | null;
  logo?: string | null;
  metadata?: Record<string, unknown> | string | null;
  name?: string | null;
  organizationId?: string;
  phone?: string | null;
  registrationNumber?: string | null;
  segment?: string | null;
  slug?: string | null;
  state?: string | null;
  status?: "active" | "inactive" | null;
  taxId?: string | null;
  timezone?: string | null;
  website?: string | null;
  zipCode?: string | null;
}

const listOrganizationsServerFn = createServerFn({ method: "GET" }).handler(
  async () => {
    const headers = getRequestHeaders();
    const organizations = await (await getAuth()).api.listOrganizations({
      headers,
    });
    return [
      ...new Map(
        (organizations ?? [])
          .map(mapOrganization)
          .map((organization) => [organization.id, organization])
      ).values(),
    ];
  }
);

const getOrganizationServerFn = createServerFn({ method: "GET" })
  .inputValidator((data: { organizationId: string }) => data)
  .handler(async ({ data }) => {
    const headers = getRequestHeaders();
    const organization = await (await getAuth()).api.getFullOrganization({
      query: { organizationId: data.organizationId },
      headers,
    });

    if (!organization) {
      return null;
    }

    return mapOrganization(organization);
  });

const createOrganizationServerFn = createServerFn({ method: "POST" })
  .inputValidator((data: { name: string; slug?: string | null }) => data)
  .handler(async ({ data }) => {
    const headers = getRequestHeaders();
    const slug =
      data.slug ??
      `${data.name.toLowerCase().replace(/\s+/g, "-")}-${Date.now().toString(36)}`;

    const organization = await (await getAuth()).api.createOrganization({
      body: {
        name: data.name,
        slug,
      },
      headers,
    });

    if (!organization) {
      throw new Error("Failed to create organization");
    }

    return mapOrganization(organization);
  });

const updateOrganizationServerFn = createServerFn({ method: "POST" })
  .inputValidator((data: UpdateOrganizationParams) => data)
  .handler(async ({ data }) => {
    const headers = getRequestHeaders();
    const organizationId = data.organizationId ?? data.id;

    if (!organizationId) {
      throw new Error("organizationId or id is required");
    }

    const {
      organizationId: _organizationId,
      id: _id,
      createdAt: _createdAt,
      name,
      slug,
      logo,
      metadata,
    } = data;

    const organization = await (await getAuth()).api.updateOrganization({
      body: {
        organizationId,
        data: {
          ...(name == null ? {} : { name }),
          ...(slug == null ? {} : { slug }),
          ...(logo == null ? {} : { logo }),
          ...(metadata
            ? {
                metadata:
                  typeof metadata === "string"
                    ? JSON.parse(metadata)
                    : metadata,
              }
            : {}),
        },
      },
      headers,
    });

    if (!organization) {
      throw new Error("Failed to update organization");
    }

    return mapOrganization(organization);
  });

/**
 * Query options for fetching a list of organizations.
 * Uses Convex query for real-time data.
 */
export const organizationsQueryOptions = () =>
  queryOptions({
    queryKey: ["organizations"],
    queryFn: () => listOrganizationsServerFn(),
    staleTime: 10 * 60 * 1000,
    gcTime: DEFAULT_GC_TIME,
    placeholderData: keepPreviousData,
  });

/**
 * Query options for fetching a single organization by its ID.
 * Uses Convex query for real-time data.
 */
export const organizationQueryOptions = (organizationId: string) =>
  queryOptions({
    queryKey: ["organizations", organizationId],
    queryFn: () => getOrganizationServerFn({ data: { organizationId } }),
    staleTime: 10 * 60 * 1000,
    gcTime: DEFAULT_GC_TIME,
  });

/**
 * Hook to fetch all organizations for the current user.
 * Uses Convex query for real-time data.
 * Returns data in OrganizationsApiResponse format for backwards compatibility.
 */
export const useOrganizations = (_props?: UseOrganizationsProps) => {
  const query = useQuery(organizationsQueryOptions());

  return {
    ...query,
    data: query.data
      ? ({
          page: query.data,
          pageCount: 1,
        } satisfies OrganizationsApiResponse)
      : undefined,
  };
};

/**
 * Hook to fetch a single organization by ID.
 * Uses Convex query for real-time data.
 */
export const useOrganization = (params: { id: string }) => {
  const query = useQuery({
    ...organizationQueryOptions(params.id),
    enabled: !!params.id,
  });

  return {
    ...query,
    data: query.data ?? undefined,
  };
};

/**
 * Hook to create a new organization.
 * Uses Convex mutation.
 */
/** Creates a new organization. */
export const useCreateOrganization = () => {
  const queryClient = useQueryClient();
  return useMutation({
    mutationKey: ["organizations", "create"],
    mutationFn: (data: { name: string; slug?: string | null }) =>
      createOrganizationServerFn({ data }),
    onSuccess: (data) => {
      getPostHog()?.capture("org_created", { org_id: data?.id });
      queryClient.invalidateQueries({
        queryKey: queryKeys.organizations._def,
      });
    },
    onError: (err) => {
      getPostHog()?.capture("mutation_error", {
        action: "org_created",
        error_message: err instanceof Error ? err.message : "Unknown error",
      });
    },
  });
};

/**
 * Hook to update an existing organization.
 * Uses Convex mutation.
 */
export const useUpdateOrganization = () => {
  const queryClient = useQueryClient();
  return useMutation({
    mutationKey: ["organizations", "update"],
    mutationFn: (params: UpdateOrganizationParams) =>
      updateOrganizationServerFn({ data: params }),
    onSuccess: (_data, variables) => {
      getPostHog()?.capture("org_updated", { org_id: variables.id });
      queryClient.invalidateQueries({
        queryKey: queryKeys.organizations._def,
      });
    },
    onError: (err) => {
      getPostHog()?.capture("mutation_error", {
        action: "org_updated",
        error_message: err instanceof Error ? err.message : "Unknown error",
      });
    },
  });
};

/**
 * Hook to set the default/active organization.
 * Uses Better Auth server function (requires server-side session management).
 */
export const useSetDefaultOrganization = () => {
  const queryClient = useQueryClient();
  return useMutation({
    mutationKey: ["organizations", "setDefault"],
    mutationFn: (data: { organizationId?: string; id?: string }) => {
      const organizationId = data.organizationId || data.id;
      if (!organizationId) {
        throw new Error("organizationId or id is required");
      }
      return setActiveOrganizationAuth({ data: { organizationId } });
    },
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: queryKeys.organizations._def,
      });
      queryClient.invalidateQueries({
        queryKey: queryKeys.projects._def,
      });
    },
  });
};

/**
 * Hook to request organization deletion.
 */
export const useRequestOrganizationDeletion = () =>
  useMutation<
    {
      success: boolean;
      message?: string;
      cooldownRemaining?: number;
    },
    Error,
    z.infer<typeof RequestOrganizationDeletionSchema>
  >({
    mutationKey: ["organizations", "requestDeletion"],
    mutationFn: (data) => requestOrganizationDeletionServerFn({ data }),
    onSuccess: (_data, variables) => {
      getPostHog()?.capture("org_deletion_requested", {
        org_id: variables.organizationId,
      });
    },
    onError: (err) => {
      getPostHog()?.capture("mutation_error", {
        action: "org_deletion_requested",
        error_message: err instanceof Error ? err.message : "Unknown error",
      });
    },
  });

/**
 * Hook to verify organization deletion code.
 */
export const useVerifyOrganizationDeletion = () =>
  useMutation<
    z.infer<typeof VerifyOrganizationDeletionResponseSchema>,
    Error,
    z.infer<typeof VerifyOrganizationDeletionSchema>
  >({
    mutationKey: ["organizations", "verify"],
    mutationFn: (data) => verifyOrganizationDeletionServerFn({ data }),
  });

/**
 * Hook to delete organization with token.
 */
export const useDeleteOrganizationWithToken = () =>
  useMutation<
    {
      success: boolean;
      message?: string;
      organizationId?: string;
      deleted?: boolean;
      user?: unknown;
    },
    Error,
    z.infer<typeof DeleteOrganizationWithTokenSchema>
  >({
    mutationKey: ["organizations", "deleteWithToken"],
    mutationFn: (data) => deleteOrganizationWithTokenServerFn({ data }),
  });

/**
 * Hook to resend organization deletion code.
 */
export const useResendOrganizationDeletionCode = () =>
  useMutation<
    z.infer<typeof ResendOrganizationDeletionCodeResponseSchema>,
    Error,
    z.infer<typeof ResendOrganizationDeletionCodeSchema>
  >({
    mutationKey: ["organizations", "resendDeletionCode"],
    mutationFn: (data) => resendOrganizationDeletionCodeServerFn({ data }),
  });

/**
 * Hook to delete the last organization with token.
 */
export const useDeleteLastOrganizationWithToken = () =>
  useMutation<
    {
      success: boolean;
      message?: string;
      organizationId?: string;
      userOnboardingReset?: boolean;
    },
    Error,
    z.infer<typeof DeleteLastOrganizationWithTokenSchema>
  >({
    mutationKey: ["organizations", "deleteLast"],
    mutationFn: (data) => deleteLastOrganizationWithTokenServerFn({ data }),
  });

export type { UpdateOrganizationParams };
