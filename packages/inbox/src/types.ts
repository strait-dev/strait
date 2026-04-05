import type { Effect } from "effect";
import type { InboxClientError } from "./errors";

export type NotifyDigestPolicy = "instant" | "hourly" | "daily";

export type InboxItemState = "unread" | "read" | "archived" | "actioned";

export interface InboxItem {
  action_result?: unknown;
  actioned_at?: string;
  actions: unknown;
  archived_at?: string;
  avatar?: string;
  body?: string;
  category_key?: string;
  created_at: string;
  dedup_count: number;
  dedup_key?: string;
  id: string;
  message_id?: string;
  priority: string;
  project_id: string;
  read_at?: string;
  recipient_id: string;
  recipient_type: string;
  state: InboxItemState;
  tenant_id?: string;
  title: string;
  updated_at: string;
  workflow_id?: string;
  workflow_run_id?: string;
}

export interface NotificationPreference {
  channel_prefs: unknown;
  created_at: string;
  critical_override: boolean;
  digest_policy: NotifyDigestPolicy;
  id: string;
  phone?: string;
  quiet_hours?: unknown;
  rate_limit_override?: number;
  recipient_id: string;
  recipient_type: string;
  scope: string;
  timezone: string;
  updated_at: string;
}

export interface ListInboxInput {
  cursor?: string;
  limit?: number;
  state?: InboxItemState;
}

export interface UnreadCountResponse {
  count: number;
}

export interface UpdateInboxItemStateInput {
  itemId: string;
  state: Extract<InboxItemState, "read" | "archived">;
}

export interface InboxActionInput {
  actionIndex: number;
  itemId: string;
}

export interface InboxActionResponse {
  item: InboxItem;
}

export interface UpdateNotifyPreferencesRequest {
  channel_prefs?: unknown;
  critical_override?: boolean;
  digest_policy?: NotifyDigestPolicy;
  phone?: string;
  quiet_hours?: unknown;
  rate_limit_override?: number;
  timezone?: string;
}

export interface ResolveUnsubscribeTokenResponse {
  expires_at: string;
  scope: string;
  subscriber_id: string;
  token: string;
}

export interface ProcessUnsubscribeRequest {
  scope?: string;
}

export interface ProcessUnsubscribeResponse {
  scope: string;
  status: "ok";
}

export type AccessTokenProvider = string | (() => string | Promise<string>);

export type FetchLike = (
  input: string | URL | Request,
  init?: RequestInit
) => Promise<Response>;

export interface InboxClientConfig {
  baseUrl: string;
  fetch?: FetchLike;
  headers?: Record<string, string>;
  token: AccessTokenProvider;
}

export interface InboxClient {
  getUnreadCount(): Effect.Effect<UnreadCountResponse, InboxClientError>;
  listInbox(
    input?: ListInboxInput
  ): Effect.Effect<InboxItem[], InboxClientError>;
  listPreferences(): Effect.Effect<NotificationPreference[], InboxClientError>;
  markAllRead(): Effect.Effect<UnreadCountResponse, InboxClientError>;
  performItemAction(
    input: InboxActionInput
  ): Effect.Effect<InboxActionResponse, InboxClientError>;
  processUnsubscribe(
    token: string,
    input?: ProcessUnsubscribeRequest
  ): Effect.Effect<ProcessUnsubscribeResponse, InboxClientError>;
  processUnsubscribeOneClick(
    token: string
  ): Effect.Effect<ProcessUnsubscribeResponse, InboxClientError>;
  resolveUnsubscribeToken(
    token: string
  ): Effect.Effect<ResolveUnsubscribeTokenResponse, InboxClientError>;
  updateItemState(
    input: UpdateInboxItemStateInput
  ): Effect.Effect<InboxItem, InboxClientError>;
  updatePreferences(
    input: UpdateNotifyPreferencesRequest
  ): Effect.Effect<NotificationPreference[], InboxClientError>;
  updatePreferencesScope(
    scope: string,
    input: UpdateNotifyPreferencesRequest
  ): Effect.Effect<NotificationPreference[], InboxClientError>;
}
