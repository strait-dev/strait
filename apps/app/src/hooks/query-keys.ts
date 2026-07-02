import { createQueryKeyStore } from "@lukemorales/query-key-factory";
import type { ListParams } from "@/hooks/api/types";

type ListJobsSearch = ListParams & { status?: string; search?: string };
type ListRunsSearch = ListParams & {
  status?: string;
  job_id?: string;
  search?: string;
};
type ListSchedulesSearch = ListParams & { status?: string };
type ListWorkflowsSearch = ListParams & { search?: string };
type ListWebhooksSearch = ListParams;
type ListEventsSearch = ListParams & {
  status?: string;
  workflow_run_id?: string;
  source_type?: string;
};
type ListDlqSearch = ListParams & { search?: string };

/**
 * Centralized query key store for the entire application.
 * All query and mutation keys should be defined here as a single source of truth.
 *
 * Usage:
 * ```ts
 * import { queryKeys } from "@/hooks/query-keys";
 *
 * // In queryOptions:
 * queryKey: queryKeys.jobs.list(search).queryKey
 *
 * // For invalidation:
 * queryClient.invalidateQueries({ queryKey: queryKeys.jobs._def })
 * ```
 */
export const queryKeys = createQueryKeyStore({
  auth: {
    accounts: null,
    passkeys: null,
    sessions: null,
  },

  users: {
    update: null,
  },

  organizations: {
    list: null,
    detail: (organizationId: string) => [organizationId],
  },

  invitations: {
    list: (organizationId: string) => [organizationId],
    detail: (invitationId: string) => [invitationId],
  },

  members: {
    list: (organizationId: string) => [organizationId],
  },

  userInvitations: {
    list: null,
  },

  subscription: {
    current: null,
    state: null,
  },

  billing: {
    orgUsage: null,
    spendingLimit: null,
    anomalyAlerts: null,
    projectCosts: null,
    usageForecast: null,
    usageHistory: null,
    downgradePreview: (targetTier: string) => [targetTier],
    projectBudget: (projectId: string) => [projectId],
    anomalyConfig: null,
    emailPreferences: null,
  },

  projects: {
    list: (organizationId: string) => [organizationId],
    detail: (id: string) => [id],
  },

  projectPermissions: {
    detail: (projectId: string) => [projectId],
  },

  onboarding: {
    complete: null,
  },

  jobs: {
    list: (search?: ListJobsSearch) => [{ search }],
    detail: (id: string) => [id],
  },

  runs: {
    list: (search?: ListRunsSearch) => [{ search }],
    detail: (id: string) => [id],
    events: (runId: string) => [runId],
  },

  schedules: {
    list: (search?: ListSchedulesSearch) => [{ search }],
  },

  workflows: {
    list: (search?: ListWorkflowsSearch) => [{ search }],
    detail: (id: string) => [id],
    steps: (workflowId: string) => [workflowId],
    runs: (workflowId: string) => [workflowId],
  },

  webhooks: {
    list: (search?: ListWebhooksSearch) => [{ search }],
    detail: (id: string) => [id],
    deliveries: (webhookId: string) => [webhookId],
  },

  events: {
    list: (search?: ListEventsSearch) => [{ search }],
    detail: (id: string) => [id],
  },

  dlq: {
    list: (search?: ListDlqSearch) => [{ search }],
  },

  apiKeys: {
    list: (search?: ListParams) => [{ search }],
  },

  oauthConsents: {
    list: null,
  },
});
