import {
  keepPreviousData,
  queryOptions,
  useMutation,
  useQueryClient,
} from "@tanstack/react-query";
import { createServerFn } from "@tanstack/react-start";
import type {
  ListParams,
  NotificationCategory,
  NotificationMessage,
  NotificationProvider,
  NotificationTemplate,
  NotifyCategoryType,
  NotifyDeliveryChannel,
  NotifyDigestPolicy,
  NotifyEscalationState,
  NotifyMessageStatus,
  NotifyPolicyOverride,
  NotifyPreference,
  NotifySubscriber,
  NotifySubscriberStatus,
  NotifySuppressionEvent,
  NotifyTopic,
  NotifyTriggerResponse,
} from "@/hooks/api/types";
import { queryKeys } from "@/hooks/query-keys";
import { DEFAULT_GC_TIME, DEFAULT_STALE_TIME } from "@/hooks/utils";
import { apiEffect, runWithSentryReport } from "@/lib/effect-api.server";
import { authMiddleware } from "@/middlewares/auth";

type NotifyDeliveriesSearch = ListParams & {
  status?: NotifyMessageStatus;
  channel?: NotifyDeliveryChannel;
  category_key?: string;
  from?: string;
  to?: string;
};
type NotifySubscribersSearch = ListParams & {
  status?: NotifySubscriberStatus;
  tenant_id?: string;
};
type NotifyTemplatesSearch = ListParams & { status?: string };
type NotifyTopicSubscribersSearch = ListParams & {
  topicKey: string;
  tenant_id?: string;
};
type NotifyPolicyScopeType = NotifyPolicyOverride["scope_type"];

type UpsertNotifySubscriberInput = {
  external_id: string;
  email?: string;
  phone?: string;
  locale?: string;
  timezone?: string;
  tenant_id?: string;
  attributes?: Record<string, object>;
};

type CreateNotifyTopicInput = {
  topic_key: string;
  name: string;
  description?: string;
  attributes?: Record<string, object>;
};

type UpsertNotifyTemplateInput = {
  template_key: string;
  name: string;
  description?: string;
  channels: Record<string, object>;
  variables?: Record<string, object>;
  locale_templates?: Record<string, object>;
  default_locale?: string;
  status?: string;
};

type UpsertNotifyCategoryInput = {
  category_key: string;
  name: string;
  description?: string;
  type?: NotifyCategoryType;
};

type UpsertNotifyProviderInput = {
  channel: NotifyDeliveryChannel;
  provider: string;
  name: string;
  config: Record<string, object>;
  is_default?: boolean;
  fallback_id?: string;
  rate_limit?: number;
};

type UpsertNotifyPolicyInput = {
  scope_type: "project" | "category" | "workflow_step";
  scope_key: string;
  channel?: NotifyDeliveryChannel;
  digest_policy?: NotifyDigestPolicy;
  retry_max_attempts?: number;
  retry_base_delay_secs?: number;
  retry_max_delay_secs?: number;
  escalation_tiers?: number;
  escalation_min_interval_secs?: number;
  enabled?: boolean;
};

type UpdateNotifyPolicyInput = Partial<
  Omit<UpsertNotifyPolicyInput, "scope_type" | "scope_key" | "channel">
> & {
  digest_policy?: NotifyDigestPolicy;
};

type NotifyRecipientInput = {
  type: "subscriber" | "topic";
  id?: string;
  key?: string;
};

type NotifyTriggerInput = {
  to: NotifyRecipientInput;
  template_key: string;
  payload?: Record<string, object>;
  channels?: NotifyDeliveryChannel[];
  category_key?: string;
  workflow_run_id?: string;
  step_run_id?: string;
  tenant_id?: string;
  dedup?: {
    key: string;
    window?: string;
  };
  schedule?: {
    delay?: string;
    at?: string;
  };
};

type UpdateNotifySubscriberPreferenceInput = {
  subscriberId: string;
  scope?: string;
  channel_prefs?: Record<string, object | string | number | boolean | null>;
  quiet_hours?: Record<string, object | string | number | boolean | null>;
  phone?: string;
  timezone?: string;
  digest_policy?: NotifyDigestPolicy;
  critical_override?: boolean;
  rate_limit_override?: number;
};

const getNotifyAPIBaseURL = () =>
  process.env.STRAIT_API_URL || "http://localhost:8080";

const issueNotifySubscriberToken = async (subscriberId: string) => {
  const tokenResponse = await runWithSentryReport(
    apiEffect<{ token: string }>(`/v1/subscribers/${subscriberId}/token`, {
      method: "POST",
      body: { expires_in: "10m" },
    })
  );

  return tokenResponse.token;
};

const callNotifySubscriberEndpoint = async <T>(
  subscriberId: string,
  path: string,
  options?: {
    method?: "GET" | "POST" | "PUT" | "PATCH" | "DELETE";
    body?: unknown;
  }
): Promise<T> => {
  const token = await issueNotifySubscriberToken(subscriberId);
  const response = await fetch(`${getNotifyAPIBaseURL()}${path}`, {
    method: options?.method ?? "GET",
    headers: {
      Authorization: `Bearer ${token}`,
      "Content-Type": "application/json",
    },
    body: options?.body ? JSON.stringify(options.body) : undefined,
  });

  if (!response.ok) {
    const text = await response.text();
    throw new Error(
      `notify subscriber request failed (${response.status}): ${text}`
    );
  }

  if (response.status === 204) {
    return {} as T;
  }

  return (await response.json()) as T;
};

export const fetchNotifyDeliveries = createServerFn({ method: "GET" })
  .inputValidator((data: NotifyDeliveriesSearch) => data)
  .middleware([authMiddleware])
  .handler(async ({ data }): Promise<NotificationMessage[]> => {
    return await runWithSentryReport(
      apiEffect<NotificationMessage[]>("/v1/notify/deliveries", {
        params: {
          status: data.status,
          channel: data.channel,
          category_key: data.category_key,
          from: data.from,
          to: data.to,
          limit: data.limit,
          cursor: data.cursor,
        },
      })
    );
  });

export const fetchNotifySubscribers = createServerFn({ method: "GET" })
  .inputValidator((data: NotifySubscribersSearch) => data)
  .middleware([authMiddleware])
  .handler(async ({ data }): Promise<NotifySubscriber[]> => {
    return await runWithSentryReport(
      apiEffect<NotifySubscriber[]>("/v1/subscribers", {
        params: {
          status: data.status,
          tenant_id: data.tenant_id,
          limit: data.limit,
          cursor: data.cursor,
        },
      })
    );
  });

export const fetchNotifySubscriber = createServerFn({ method: "GET" })
  .inputValidator((data: { subscriberId: string }) => data)
  .middleware([authMiddleware])
  .handler(async ({ data }): Promise<NotifySubscriber> => {
    return await runWithSentryReport(
      apiEffect<NotifySubscriber>(`/v1/subscribers/${data.subscriberId}`)
    );
  });

export const createNotifySubscriberFn = createServerFn({ method: "POST" })
  .inputValidator((data: UpsertNotifySubscriberInput) => data)
  .middleware([authMiddleware])
  .handler(async ({ data }): Promise<NotifySubscriber> => {
    return await runWithSentryReport(
      apiEffect<NotifySubscriber>("/v1/subscribers", {
        method: "POST",
        body: data,
      })
    );
  });

export const updateNotifySubscriberFn = createServerFn({ method: "POST" })
  .inputValidator(
    (data: UpsertNotifySubscriberInput & { subscriberId: string }) => data
  )
  .middleware([authMiddleware])
  .handler(async ({ data }): Promise<NotifySubscriber> => {
    const { subscriberId, ...body } = data;
    return await runWithSentryReport(
      apiEffect<NotifySubscriber>(`/v1/subscribers/${subscriberId}`, {
        method: "PUT",
        body,
      })
    );
  });

export const deleteNotifySubscriberFn = createServerFn({ method: "POST" })
  .inputValidator((data: { subscriberId: string; purge?: boolean }) => data)
  .middleware([authMiddleware])
  .handler(async ({ data }): Promise<void> => {
    return await runWithSentryReport(
      apiEffect<void>(`/v1/subscribers/${data.subscriberId}`, {
        method: "DELETE",
        params: { purge: data.purge },
      })
    );
  });

export const createNotifySubscriberTokenFn = createServerFn({ method: "POST" })
  .inputValidator(
    (data: { subscriberId: string; expires_in?: string; tenant_id?: string }) =>
      data
  )
  .middleware([authMiddleware])
  .handler(async ({ data }): Promise<{ token: string }> => {
    const { subscriberId, ...body } = data;
    return await runWithSentryReport(
      apiEffect<{ token: string }>(`/v1/subscribers/${subscriberId}/token`, {
        method: "POST",
        body,
      })
    );
  });

export const listNotifySuppressionEventsFn = createServerFn({ method: "GET" })
  .inputValidator((data: { subscriberId: string } & ListParams) => data)
  .middleware([authMiddleware])
  .handler(async ({ data }): Promise<NotifySuppressionEvent[]> => {
    return await runWithSentryReport(
      apiEffect<NotifySuppressionEvent[]>(
        `/v1/subscribers/${data.subscriberId}/suppressions`,
        {
          params: {
            limit: data.limit,
            cursor: data.cursor,
          },
        }
      )
    );
  });

export const fetchNotifySubscriberPreferences = createServerFn({
  method: "GET",
})
  .inputValidator((data: { subscriberId: string }) => data)
  .middleware([authMiddleware])
  .handler(async ({ data }): Promise<NotifyPreference[]> => {
    return await callNotifySubscriberEndpoint<NotifyPreference[]>(
      data.subscriberId,
      "/v1/preferences"
    );
  });

export const updateNotifySubscriberPreferenceFn = createServerFn({
  method: "POST",
})
  .inputValidator((data: UpdateNotifySubscriberPreferenceInput) => data)
  .middleware([authMiddleware])
  .handler(async ({ data }): Promise<NotifyPreference[]> => {
    const { subscriberId, scope, ...body } = data;
    const targetScope = scope || "global";

    return await callNotifySubscriberEndpoint<NotifyPreference[]>(
      subscriberId,
      `/v1/preferences/${targetScope}`,
      {
        method: "PUT",
        body,
      }
    );
  });

export const createNotifyUnsuppressFn = createServerFn({ method: "POST" })
  .inputValidator(
    (data: {
      subscriberId: string;
      channel: NotifyDeliveryChannel;
      reason?: string;
      scope?: string;
      force?: boolean;
    }) => data
  )
  .middleware([authMiddleware])
  .handler(async ({ data }): Promise<Record<string, object>> => {
    const { subscriberId, ...body } = data;
    return await runWithSentryReport(
      apiEffect<Record<string, object>>(
        `/v1/subscribers/${subscriberId}/suppressions/unsuppress`,
        {
          method: "POST",
          body,
        }
      )
    );
  });

export const fetchNotifyTopics = createServerFn({ method: "GET" })
  .middleware([authMiddleware])
  .handler(async (): Promise<NotifyTopic[]> => {
    return await runWithSentryReport(apiEffect<NotifyTopic[]>("/v1/topics"));
  });

export const fetchNotifyTopicSubscribers = createServerFn({ method: "GET" })
  .inputValidator((data: NotifyTopicSubscribersSearch) => data)
  .middleware([authMiddleware])
  .handler(async ({ data }): Promise<NotifySubscriber[]> => {
    return await runWithSentryReport(
      apiEffect<NotifySubscriber[]>(`/v1/topics/${data.topicKey}/subscribers`, {
        params: {
          tenant_id: data.tenant_id,
          limit: data.limit,
        },
      })
    );
  });

export const createNotifyTopicFn = createServerFn({ method: "POST" })
  .inputValidator((data: CreateNotifyTopicInput) => data)
  .middleware([authMiddleware])
  .handler(async ({ data }): Promise<NotifyTopic> => {
    return await runWithSentryReport(
      apiEffect<NotifyTopic>("/v1/topics", {
        method: "POST",
        body: data,
      })
    );
  });

export const addNotifyTopicSubscriberFn = createServerFn({ method: "POST" })
  .inputValidator((data: { topicKey: string; subscriber_id: string }) => data)
  .middleware([authMiddleware])
  .handler(async ({ data }): Promise<void> => {
    return await runWithSentryReport(
      apiEffect<void>(`/v1/topics/${data.topicKey}/subscribers`, {
        method: "POST",
        body: { subscriber_id: data.subscriber_id },
      })
    );
  });

export const removeNotifyTopicSubscriberFn = createServerFn({ method: "POST" })
  .inputValidator((data: { topicKey: string; subscriberId: string }) => data)
  .middleware([authMiddleware])
  .handler(async ({ data }): Promise<void> => {
    return await runWithSentryReport(
      apiEffect<void>(
        `/v1/topics/${data.topicKey}/subscribers/${data.subscriberId}`,
        {
          method: "DELETE",
        }
      )
    );
  });

export const fetchNotificationTemplates = createServerFn({ method: "GET" })
  .inputValidator((data: NotifyTemplatesSearch) => data)
  .middleware([authMiddleware])
  .handler(async ({ data }): Promise<NotificationTemplate[]> => {
    return await runWithSentryReport(
      apiEffect<NotificationTemplate[]>("/v1/templates", {
        params: {
          status: data.status,
          limit: data.limit,
          cursor: data.cursor,
        },
      })
    );
  });

export const fetchNotificationTemplate = createServerFn({ method: "GET" })
  .inputValidator((data: { templateKey: string }) => data)
  .middleware([authMiddleware])
  .handler(async ({ data }): Promise<NotificationTemplate> => {
    return await runWithSentryReport(
      apiEffect<NotificationTemplate>(`/v1/templates/${data.templateKey}`)
    );
  });

export const createNotificationTemplateFn = createServerFn({ method: "POST" })
  .inputValidator((data: UpsertNotifyTemplateInput) => data)
  .middleware([authMiddleware])
  .handler(async ({ data }): Promise<NotificationTemplate> => {
    return await runWithSentryReport(
      apiEffect<NotificationTemplate>("/v1/templates", {
        method: "POST",
        body: data,
      })
    );
  });

export const updateNotificationTemplateFn = createServerFn({ method: "POST" })
  .inputValidator(
    (data: UpsertNotifyTemplateInput & { templateKey: string }) => data
  )
  .middleware([authMiddleware])
  .handler(async ({ data }): Promise<NotificationTemplate> => {
    const { templateKey, ...body } = data;
    return await runWithSentryReport(
      apiEffect<NotificationTemplate>(`/v1/templates/${templateKey}`, {
        method: "PUT",
        body,
      })
    );
  });

export const fetchNotificationCategories = createServerFn({ method: "GET" })
  .middleware([authMiddleware])
  .handler(async (): Promise<NotificationCategory[]> => {
    return await runWithSentryReport(
      apiEffect<NotificationCategory[]>("/v1/categories")
    );
  });

export const createNotificationCategoryFn = createServerFn({ method: "POST" })
  .inputValidator((data: UpsertNotifyCategoryInput) => data)
  .middleware([authMiddleware])
  .handler(async ({ data }): Promise<NotificationCategory> => {
    return await runWithSentryReport(
      apiEffect<NotificationCategory>("/v1/categories", {
        method: "POST",
        body: data,
      })
    );
  });

export const fetchNotificationProviders = createServerFn({ method: "GET" })
  .inputValidator((data: { channel?: NotifyDeliveryChannel }) => data)
  .middleware([authMiddleware])
  .handler(async ({ data }): Promise<NotificationProvider[]> => {
    return await runWithSentryReport(
      apiEffect<NotificationProvider[]>("/v1/providers", {
        params: { channel: data.channel },
      })
    );
  });

export const createNotificationProviderFn = createServerFn({ method: "POST" })
  .inputValidator((data: UpsertNotifyProviderInput) => data)
  .middleware([authMiddleware])
  .handler(async ({ data }): Promise<NotificationProvider> => {
    return await runWithSentryReport(
      apiEffect<NotificationProvider>("/v1/providers", {
        method: "POST",
        body: data,
      })
    );
  });

export const updateNotificationProviderFn = createServerFn({ method: "POST" })
  .inputValidator(
    (data: UpsertNotifyProviderInput & { providerId: string }) => data
  )
  .middleware([authMiddleware])
  .handler(async ({ data }): Promise<NotificationProvider> => {
    const { providerId, ...body } = data;
    return await runWithSentryReport(
      apiEffect<NotificationProvider>(`/v1/providers/${providerId}`, {
        method: "PUT",
        body,
      })
    );
  });

export const deleteNotificationProviderFn = createServerFn({ method: "POST" })
  .inputValidator((data: { providerId: string }) => data)
  .middleware([authMiddleware])
  .handler(async ({ data }): Promise<void> => {
    return await runWithSentryReport(
      apiEffect<void>(`/v1/providers/${data.providerId}`, { method: "DELETE" })
    );
  });

export const fetchNotifyPolicyOverrides = createServerFn({ method: "GET" })
  .inputValidator((data: { scope_type?: NotifyPolicyScopeType }) => data)
  .middleware([authMiddleware])
  .handler(async ({ data }): Promise<NotifyPolicyOverride[]> => {
    return await runWithSentryReport(
      apiEffect<NotifyPolicyOverride[]>("/v1/notify/policies", {
        params: { scope_type: data.scope_type },
      })
    );
  });

export const fetchNotifyPolicyOverride = createServerFn({ method: "GET" })
  .inputValidator((data: { policyId: string }) => data)
  .middleware([authMiddleware])
  .handler(async ({ data }): Promise<NotifyPolicyOverride> => {
    return await runWithSentryReport(
      apiEffect<NotifyPolicyOverride>(`/v1/notify/policies/${data.policyId}`)
    );
  });

export const createNotifyPolicyOverrideFn = createServerFn({ method: "POST" })
  .inputValidator((data: UpsertNotifyPolicyInput) => data)
  .middleware([authMiddleware])
  .handler(async ({ data }): Promise<NotifyPolicyOverride> => {
    return await runWithSentryReport(
      apiEffect<NotifyPolicyOverride>("/v1/notify/policies", {
        method: "POST",
        body: data,
      })
    );
  });

export const updateNotifyPolicyOverrideFn = createServerFn({ method: "POST" })
  .inputValidator(
    (data: UpdateNotifyPolicyInput & { policyId: string }) => data
  )
  .middleware([authMiddleware])
  .handler(async ({ data }): Promise<NotifyPolicyOverride> => {
    const { policyId, ...body } = data;
    return await runWithSentryReport(
      apiEffect<NotifyPolicyOverride>(`/v1/notify/policies/${policyId}`, {
        method: "PUT",
        body,
      })
    );
  });

export const deleteNotifyPolicyOverrideFn = createServerFn({ method: "POST" })
  .inputValidator((data: { policyId: string }) => data)
  .middleware([authMiddleware])
  .handler(async ({ data }): Promise<void> => {
    return await runWithSentryReport(
      apiEffect<void>(`/v1/notify/policies/${data.policyId}`, {
        method: "DELETE",
      })
    );
  });

export const notifyPreviewFn = createServerFn({ method: "POST" })
  .inputValidator(
    (data: {
      template_key: string;
      payload?: Record<string, object>;
      subscriber_id?: string;
      locale?: string;
      channels?: NotifyDeliveryChannel[];
      category_key?: string;
      unsubscribe_scope?: string;
    }) => data
  )
  .middleware([authMiddleware])
  .handler(async ({ data }): Promise<Record<string, object>> => {
    return await runWithSentryReport(
      apiEffect<Record<string, object>>("/v1/notify/preview", {
        method: "POST",
        body: data,
      })
    );
  });

export const notifyTriggerFn = createServerFn({ method: "POST" })
  .inputValidator((data: NotifyTriggerInput) => data)
  .middleware([authMiddleware])
  .handler(async ({ data }): Promise<NotifyTriggerResponse> => {
    return await runWithSentryReport(
      apiEffect<NotifyTriggerResponse>("/v1/notify", {
        method: "POST",
        body: data,
      })
    );
  });

export const notifyTestFn = createServerFn({ method: "POST" })
  .inputValidator((data: NotifyTriggerInput) => data)
  .middleware([authMiddleware])
  .handler(async ({ data }): Promise<NotifyTriggerResponse> => {
    return await runWithSentryReport(
      apiEffect<NotifyTriggerResponse>("/v1/notify/test", {
        method: "POST",
        body: data,
      })
    );
  });

export const fetchNotifyEscalationByStepRun = createServerFn({ method: "GET" })
  .inputValidator((data: { stepRunId: string }) => data)
  .middleware([authMiddleware])
  .handler(async ({ data }): Promise<NotifyEscalationState> => {
    return await runWithSentryReport(
      apiEffect<NotifyEscalationState>(
        `/v1/notify/escalations/step-runs/${data.stepRunId}`
      )
    );
  });

export const acknowledgeNotifyEscalationFn = createServerFn({ method: "POST" })
  .inputValidator((data: { stepRunId: string }) => data)
  .middleware([authMiddleware])
  .handler(async ({ data }): Promise<Record<string, object>> => {
    return await runWithSentryReport(
      apiEffect<Record<string, object>>(
        `/v1/notify/escalations/step-runs/${data.stepRunId}/acknowledge`,
        {
          method: "POST",
        }
      )
    );
  });

export const completeNotifyEscalationFn = createServerFn({ method: "POST" })
  .inputValidator(
    (data: { stepRunId: string; status?: "completed" | "failed" }) => data
  )
  .middleware([authMiddleware])
  .handler(async ({ data }): Promise<Record<string, object>> => {
    return await runWithSentryReport(
      apiEffect<Record<string, object>>(
        `/v1/notify/escalations/step-runs/${data.stepRunId}/complete`,
        {
          method: "POST",
          body: { status: data.status },
        }
      )
    );
  });

export const notifyDeliveriesQueryOptions = (search?: NotifyDeliveriesSearch) =>
  queryOptions({
    queryKey: queryKeys.notify.deliveries(search).queryKey,
    queryFn: () => fetchNotifyDeliveries({ data: search ?? {} }),
    staleTime: DEFAULT_STALE_TIME,
    gcTime: DEFAULT_GC_TIME,
    placeholderData: keepPreviousData,
  });

export const notifySubscribersQueryOptions = (
  search?: NotifySubscribersSearch
) =>
  queryOptions({
    queryKey: queryKeys.notify.subscribersList(search).queryKey,
    queryFn: () => fetchNotifySubscribers({ data: search ?? {} }),
    staleTime: DEFAULT_STALE_TIME,
    gcTime: DEFAULT_GC_TIME,
    placeholderData: keepPreviousData,
  });

export const notifySubscriberQueryOptions = (subscriberId: string) =>
  queryOptions({
    queryKey: queryKeys.notify.subscriberDetail(subscriberId).queryKey,
    queryFn: () => fetchNotifySubscriber({ data: { subscriberId } }),
    enabled: !!subscriberId,
    staleTime: DEFAULT_STALE_TIME,
    gcTime: DEFAULT_GC_TIME,
  });

export const notifySubscriberSuppressionsQueryOptions = (
  subscriberId: string,
  search?: ListParams
) =>
  queryOptions({
    queryKey: queryKeys.notify.subscriberSuppressions(subscriberId, search)
      .queryKey,
    queryFn: () =>
      listNotifySuppressionEventsFn({
        data: { subscriberId, ...(search ?? {}) },
      }),
    enabled: !!subscriberId,
    staleTime: DEFAULT_STALE_TIME,
    gcTime: DEFAULT_GC_TIME,
    placeholderData: keepPreviousData,
  });

export const notifySubscriberPreferencesQueryOptions = (subscriberId: string) =>
  queryOptions({
    queryKey: queryKeys.notify.subscriberPreferences(subscriberId).queryKey,
    queryFn: () => fetchNotifySubscriberPreferences({ data: { subscriberId } }),
    enabled: !!subscriberId,
    staleTime: DEFAULT_STALE_TIME,
    gcTime: DEFAULT_GC_TIME,
  });

export const notifyTopicsQueryOptions = () =>
  queryOptions({
    queryKey: queryKeys.notify.topics.queryKey,
    queryFn: () => fetchNotifyTopics(),
    staleTime: DEFAULT_STALE_TIME,
    gcTime: DEFAULT_GC_TIME,
  });

export const notifyTopicSubscribersQueryOptions = (
  search?: NotifyTopicSubscribersSearch
) =>
  queryOptions({
    queryKey: queryKeys.notify.topicSubscribers(search).queryKey,
    queryFn: () => {
      if (!search?.topicKey) {
        return Promise.resolve([] as NotifySubscriber[]);
      }

      return fetchNotifyTopicSubscribers({ data: search });
    },
    enabled: !!search?.topicKey,
    staleTime: DEFAULT_STALE_TIME,
    gcTime: DEFAULT_GC_TIME,
    placeholderData: keepPreviousData,
  });

export const notifyTemplatesQueryOptions = (search?: NotifyTemplatesSearch) =>
  queryOptions({
    queryKey: queryKeys.notify.templatesList(search).queryKey,
    queryFn: () => fetchNotificationTemplates({ data: search ?? {} }),
    staleTime: DEFAULT_STALE_TIME,
    gcTime: DEFAULT_GC_TIME,
    placeholderData: keepPreviousData,
  });

export const notifyTemplateQueryOptions = (templateKey: string) =>
  queryOptions({
    queryKey: queryKeys.notify.templateDetail(templateKey).queryKey,
    queryFn: () => fetchNotificationTemplate({ data: { templateKey } }),
    enabled: !!templateKey,
    staleTime: DEFAULT_STALE_TIME,
    gcTime: DEFAULT_GC_TIME,
  });

export const notifyCategoriesQueryOptions = () =>
  queryOptions({
    queryKey: queryKeys.notify.categories.queryKey,
    queryFn: () => fetchNotificationCategories(),
    staleTime: DEFAULT_STALE_TIME,
    gcTime: DEFAULT_GC_TIME,
  });

export const notifyProvidersQueryOptions = (channel?: NotifyDeliveryChannel) =>
  queryOptions({
    queryKey: queryKeys.notify.providers(channel).queryKey,
    queryFn: () => fetchNotificationProviders({ data: { channel } }),
    staleTime: DEFAULT_STALE_TIME,
    gcTime: DEFAULT_GC_TIME,
  });

export const notifyPoliciesQueryOptions = (
  scope_type?: NotifyPolicyScopeType
) =>
  queryOptions({
    queryKey: queryKeys.notify.policiesList({ scope_type }).queryKey,
    queryFn: () => fetchNotifyPolicyOverrides({ data: { scope_type } }),
    staleTime: DEFAULT_STALE_TIME,
    gcTime: DEFAULT_GC_TIME,
  });

export const notifyPolicyQueryOptions = (policyId: string) =>
  queryOptions({
    queryKey: queryKeys.notify.policyDetail(policyId).queryKey,
    queryFn: () => fetchNotifyPolicyOverride({ data: { policyId } }),
    enabled: !!policyId,
    staleTime: DEFAULT_STALE_TIME,
    gcTime: DEFAULT_GC_TIME,
  });

export const notifyEscalationQueryOptions = (stepRunId: string) =>
  queryOptions({
    queryKey: queryKeys.notify.escalationDetail(stepRunId).queryKey,
    queryFn: () => fetchNotifyEscalationByStepRun({ data: { stepRunId } }),
    enabled: !!stepRunId,
    staleTime: DEFAULT_STALE_TIME,
    gcTime: DEFAULT_GC_TIME,
  });

const invalidateNotifyQueries = async (
  queryClient: ReturnType<typeof useQueryClient>
) => {
  await Promise.all([
    queryClient.invalidateQueries({ queryKey: queryKeys.notify._def }),
    queryClient.invalidateQueries({
      queryKey: queryKeys.notify.subscribersList._def,
    }),
  ]);
};

export const useCreateNotifySubscriber = () => {
  const queryClient = useQueryClient();
  return useMutation({
    mutationKey: ["notify", "subscribers", "create"],
    mutationFn: (data: UpsertNotifySubscriberInput) =>
      createNotifySubscriberFn({ data }),
    onSuccess: async () => {
      await invalidateNotifyQueries(queryClient);
    },
  });
};

export const useUpdateNotifySubscriber = () => {
  const queryClient = useQueryClient();
  return useMutation({
    mutationKey: ["notify", "subscribers", "update"],
    mutationFn: (
      data: UpsertNotifySubscriberInput & { subscriberId: string }
    ) => updateNotifySubscriberFn({ data }),
    onSuccess: async (_data, variables) => {
      await Promise.all([
        invalidateNotifyQueries(queryClient),
        queryClient.invalidateQueries({
          queryKey: queryKeys.notify.subscriberDetail(variables.subscriberId)
            .queryKey,
        }),
      ]);
    },
  });
};

export const useDeleteNotifySubscriber = () => {
  const queryClient = useQueryClient();
  return useMutation({
    mutationKey: ["notify", "subscribers", "delete"],
    mutationFn: (data: { subscriberId: string; purge?: boolean }) =>
      deleteNotifySubscriberFn({ data }),
    onSuccess: async () => {
      await invalidateNotifyQueries(queryClient);
    },
  });
};

export const useUpdateNotifySubscriberPreference = () => {
  const queryClient = useQueryClient();
  return useMutation({
    mutationKey: ["notify", "subscribers", "preferences", "update"],
    mutationFn: (data: UpdateNotifySubscriberPreferenceInput) =>
      updateNotifySubscriberPreferenceFn({ data }),
    onSuccess: async (_data, variables) => {
      await queryClient.invalidateQueries({
        queryKey: queryKeys.notify.subscriberPreferences(variables.subscriberId)
          .queryKey,
      });
    },
  });
};

export const useNotifyUnsuppressSubscriber = () => {
  const queryClient = useQueryClient();
  return useMutation({
    mutationKey: ["notify", "subscribers", "unsuppress"],
    mutationFn: (data: {
      subscriberId: string;
      channel: NotifyDeliveryChannel;
      reason?: string;
      scope?: string;
      force?: boolean;
    }) => createNotifyUnsuppressFn({ data }),
    onSuccess: async (_data, variables) => {
      await Promise.all([
        queryClient.invalidateQueries({
          queryKey: queryKeys.notify.subscriberSuppressions(
            variables.subscriberId,
            undefined
          ).queryKey,
        }),
        queryClient.invalidateQueries({
          queryKey: queryKeys.notify.deliveries._def,
        }),
      ]);
    },
  });
};

export const useCreateNotifyTopic = () => {
  const queryClient = useQueryClient();
  return useMutation({
    mutationKey: ["notify", "topics", "create"],
    mutationFn: (data: CreateNotifyTopicInput) => createNotifyTopicFn({ data }),
    onSuccess: async () => {
      await queryClient.invalidateQueries({
        queryKey: queryKeys.notify.topics.queryKey,
      });
    },
  });
};

export const useAddNotifyTopicSubscriber = () => {
  const queryClient = useQueryClient();
  return useMutation({
    mutationKey: ["notify", "topics", "add-subscriber"],
    mutationFn: (data: { topicKey: string; subscriber_id: string }) =>
      addNotifyTopicSubscriberFn({ data }),
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({
          queryKey: queryKeys.notify.topics.queryKey,
        }),
        queryClient.invalidateQueries({
          queryKey: queryKeys.notify.topicSubscribers._def,
        }),
      ]);
    },
  });
};

export const useRemoveNotifyTopicSubscriber = () => {
  const queryClient = useQueryClient();
  return useMutation({
    mutationKey: ["notify", "topics", "remove-subscriber"],
    mutationFn: (data: { topicKey: string; subscriberId: string }) =>
      removeNotifyTopicSubscriberFn({ data }),
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({
          queryKey: queryKeys.notify.topics.queryKey,
        }),
        queryClient.invalidateQueries({
          queryKey: queryKeys.notify.topicSubscribers._def,
        }),
      ]);
    },
  });
};

export const useCreateNotificationTemplate = () => {
  const queryClient = useQueryClient();
  return useMutation({
    mutationKey: ["notify", "templates", "create"],
    mutationFn: (data: UpsertNotifyTemplateInput) =>
      createNotificationTemplateFn({ data }),
    onSuccess: async () => {
      await queryClient.invalidateQueries({
        queryKey: queryKeys.notify.templatesList._def,
      });
    },
  });
};

export const useUpdateNotificationTemplate = () => {
  const queryClient = useQueryClient();
  return useMutation({
    mutationKey: ["notify", "templates", "update"],
    mutationFn: (data: UpsertNotifyTemplateInput & { templateKey: string }) =>
      updateNotificationTemplateFn({ data }),
    onSuccess: async (_data, variables) => {
      await Promise.all([
        queryClient.invalidateQueries({
          queryKey: queryKeys.notify.templatesList._def,
        }),
        queryClient.invalidateQueries({
          queryKey: queryKeys.notify.templateDetail(variables.templateKey)
            .queryKey,
        }),
      ]);
    },
  });
};

export const useCreateNotificationCategory = () => {
  const queryClient = useQueryClient();
  return useMutation({
    mutationKey: ["notify", "categories", "create"],
    mutationFn: (data: UpsertNotifyCategoryInput) =>
      createNotificationCategoryFn({ data }),
    onSuccess: async () => {
      await queryClient.invalidateQueries({
        queryKey: queryKeys.notify.categories.queryKey,
      });
    },
  });
};

export const useCreateNotificationProvider = () => {
  const queryClient = useQueryClient();
  return useMutation({
    mutationKey: ["notify", "providers", "create"],
    mutationFn: (data: UpsertNotifyProviderInput) =>
      createNotificationProviderFn({ data }),
    onSuccess: async () => {
      await queryClient.invalidateQueries({
        queryKey: queryKeys.notify.providers._def,
      });
    },
  });
};

export const useUpdateNotificationProvider = () => {
  const queryClient = useQueryClient();
  return useMutation({
    mutationKey: ["notify", "providers", "update"],
    mutationFn: (data: UpsertNotifyProviderInput & { providerId: string }) =>
      updateNotificationProviderFn({ data }),
    onSuccess: async () => {
      await queryClient.invalidateQueries({
        queryKey: queryKeys.notify.providers._def,
      });
    },
  });
};

export const useDeleteNotificationProvider = () => {
  const queryClient = useQueryClient();
  return useMutation({
    mutationKey: ["notify", "providers", "delete"],
    mutationFn: (data: { providerId: string }) =>
      deleteNotificationProviderFn({ data }),
    onSuccess: async () => {
      await queryClient.invalidateQueries({
        queryKey: queryKeys.notify.providers._def,
      });
    },
  });
};

export const useCreateNotifyPolicyOverride = () => {
  const queryClient = useQueryClient();
  return useMutation({
    mutationKey: ["notify", "policies", "create"],
    mutationFn: (data: UpsertNotifyPolicyInput) =>
      createNotifyPolicyOverrideFn({ data }),
    onSuccess: async () => {
      await queryClient.invalidateQueries({
        queryKey: queryKeys.notify.policiesList._def,
      });
    },
  });
};

export const useUpdateNotifyPolicyOverride = () => {
  const queryClient = useQueryClient();
  return useMutation({
    mutationKey: ["notify", "policies", "update"],
    mutationFn: (data: UpdateNotifyPolicyInput & { policyId: string }) =>
      updateNotifyPolicyOverrideFn({ data }),
    onSuccess: async (_data, variables) => {
      await Promise.all([
        queryClient.invalidateQueries({
          queryKey: queryKeys.notify.policiesList._def,
        }),
        queryClient.invalidateQueries({
          queryKey: queryKeys.notify.policyDetail(variables.policyId).queryKey,
        }),
      ]);
    },
  });
};

export const useDeleteNotifyPolicyOverride = () => {
  const queryClient = useQueryClient();
  return useMutation({
    mutationKey: ["notify", "policies", "delete"],
    mutationFn: (data: { policyId: string }) =>
      deleteNotifyPolicyOverrideFn({ data }),
    onSuccess: async () => {
      await queryClient.invalidateQueries({
        queryKey: queryKeys.notify.policiesList._def,
      });
    },
  });
};

export const useNotifyPreview = () => {
  const queryClient = useQueryClient();
  return useMutation({
    mutationKey: ["notify", "preview"],
    mutationFn: (data: {
      template_key: string;
      payload?: Record<string, object>;
      subscriber_id?: string;
      locale?: string;
      channels?: NotifyDeliveryChannel[];
      category_key?: string;
      unsubscribe_scope?: string;
    }) => notifyPreviewFn({ data }),
    onSuccess: async () => {
      await queryClient.invalidateQueries({
        queryKey: queryKeys.notify.preview.queryKey,
      });
    },
  });
};

export const useNotifyTrigger = () => {
  const queryClient = useQueryClient();
  return useMutation({
    mutationKey: ["notify", "trigger"],
    mutationFn: (data: NotifyTriggerInput) => notifyTriggerFn({ data }),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: queryKeys.notify._def });
    },
  });
};

export const useNotifyTest = () => {
  const queryClient = useQueryClient();
  return useMutation({
    mutationKey: ["notify", "test"],
    mutationFn: (data: NotifyTriggerInput) => notifyTestFn({ data }),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: queryKeys.notify._def });
    },
  });
};

export const useAcknowledgeNotifyEscalation = () => {
  const queryClient = useQueryClient();
  return useMutation({
    mutationKey: ["notify", "escalations", "ack"],
    mutationFn: (data: { stepRunId: string }) =>
      acknowledgeNotifyEscalationFn({ data }),
    onSuccess: async (_data, variables) => {
      await queryClient.invalidateQueries({
        queryKey: queryKeys.notify.escalationDetail(variables.stepRunId)
          .queryKey,
      });
    },
  });
};

export const useCompleteNotifyEscalation = () => {
  const queryClient = useQueryClient();
  return useMutation({
    mutationKey: ["notify", "escalations", "complete"],
    mutationFn: (data: {
      stepRunId: string;
      status?: "completed" | "failed";
    }) => completeNotifyEscalationFn({ data }),
    onSuccess: async (_data, variables) => {
      await queryClient.invalidateQueries({
        queryKey: queryKeys.notify.escalationDetail(variables.stepRunId)
          .queryKey,
      });
    },
  });
};
