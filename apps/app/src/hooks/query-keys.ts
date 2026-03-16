import { createQueryKeyStore } from "@lukemorales/query-key-factory";
import type { ListParams } from "@/hooks/api/types";

// ---------------------------------------------------------------------------
// Search / filter types used in parameterized keys
// ---------------------------------------------------------------------------

type ListJobsSearch = ListParams & { status?: string };
type ListRunsSearch = ListParams & { status?: string; job_id?: string };
type ListSchedulesSearch = ListParams;
type ListWorkflowsSearch = ListParams;
type ListWebhooksSearch = ListParams;
type ListEventsSearch = ListParams & { type?: string };
type ListDlqSearch = ListParams;

// ---------------------------------------------------------------------------
// Query Key Store
// ---------------------------------------------------------------------------

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
  // ── Auth ──────────────────────────────────────────────────────────────────
  auth: {
    accounts: null,
    passkeys: null,
    sessions: null,
  },

  // ── Users ─────────────────────────────────────────────────────────────────
  users: {
    update: null,
  },

  // ── Organizations ─────────────────────────────────────────────────────────
  organizations: {
    list: null,
    detail: (organizationId: string) => [organizationId],
  },

  // ── Invitations ───────────────────────────────────────────────────────────
  invitations: {
    list: (organizationId: string) => [organizationId],
    detail: (invitationId: string) => [invitationId],
  },

  // ── Subscription ──────────────────────────────────────────────────────────
  subscription: {
    current: null,
    state: null,
  },

  // ── Onboarding ────────────────────────────────────────────────────────────
  onboarding: {
    complete: null,
  },

  // ── Jobs ──────────────────────────────────────────────────────────────────
  jobs: {
    list: (search?: ListJobsSearch) => [{ search }],
    detail: (id: string) => [id],
  },

  // ── Runs ──────────────────────────────────────────────────────────────────
  runs: {
    list: (search?: ListRunsSearch) => [{ search }],
    detail: (id: string) => [id],
    events: (runId: string) => [runId],
  },

  // ── Schedules ─────────────────────────────────────────────────────────────
  schedules: {
    list: (search?: ListSchedulesSearch) => [{ search }],
  },

  // ── Workflows ─────────────────────────────────────────────────────────────
  workflows: {
    list: (search?: ListWorkflowsSearch) => [{ search }],
    detail: (id: string) => [id],
    steps: (workflowId: string) => [workflowId],
    runs: (workflowId: string) => [workflowId],
  },

  // ── Webhooks ──────────────────────────────────────────────────────────────
  webhooks: {
    list: (search?: ListWebhooksSearch) => [{ search }],
    detail: (id: string) => [id],
    deliveries: (webhookId: string) => [webhookId],
  },

  // ── Events ────────────────────────────────────────────────────────────────
  events: {
    list: (search?: ListEventsSearch) => [{ search }],
    detail: (id: string) => [id],
  },

  // ── DLQ ───────────────────────────────────────────────────────────────────
  dlq: {
    list: (search?: ListDlqSearch) => [{ search }],
  },
});
