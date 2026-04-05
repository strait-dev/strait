import { Schema } from "effect";

export const NotifyDigestPolicySchema = Schema.Literal(
  "instant",
  "hourly",
  "daily"
);

export const InboxItemStateSchema = Schema.Literal(
  "unread",
  "read",
  "archived",
  "actioned"
);

export const InboxItemSchema = Schema.mutable(
  Schema.Struct({
    id: Schema.String,
    recipient_type: Schema.String,
    recipient_id: Schema.String,
    project_id: Schema.String,
    tenant_id: Schema.optional(Schema.String),
    workflow_id: Schema.optional(Schema.String),
    workflow_run_id: Schema.optional(Schema.String),
    message_id: Schema.optional(Schema.String),
    category_key: Schema.optional(Schema.String),
    title: Schema.String,
    body: Schema.optional(Schema.String),
    avatar: Schema.optional(Schema.String),
    priority: Schema.String,
    state: InboxItemStateSchema,
    actions: Schema.Unknown,
    dedup_key: Schema.optional(Schema.String),
    dedup_count: Schema.Number,
    read_at: Schema.optional(Schema.String),
    archived_at: Schema.optional(Schema.String),
    actioned_at: Schema.optional(Schema.String),
    action_result: Schema.optional(Schema.Unknown),
    created_at: Schema.String,
    updated_at: Schema.String,
  })
);

export const InboxItemListSchema = Schema.mutable(
  Schema.Array(InboxItemSchema)
);

export const NotificationPreferenceSchema = Schema.mutable(
  Schema.Struct({
    id: Schema.String,
    recipient_type: Schema.String,
    recipient_id: Schema.String,
    scope: Schema.String,
    channel_prefs: Schema.Unknown,
    quiet_hours: Schema.optional(Schema.Unknown),
    phone: Schema.optional(Schema.String),
    timezone: Schema.String,
    digest_policy: NotifyDigestPolicySchema,
    critical_override: Schema.Boolean,
    rate_limit_override: Schema.optional(Schema.Number),
    created_at: Schema.String,
    updated_at: Schema.String,
  })
);

export const UnreadCountResponseSchema = Schema.Struct({
  count: Schema.Number,
});

export const InboxActionResponseSchema = Schema.Struct({
  item: InboxItemSchema,
});

export const NotificationPreferenceListSchema = Schema.mutable(
  Schema.Array(NotificationPreferenceSchema)
);

export const ResolveUnsubscribeTokenResponseSchema = Schema.Struct({
  token: Schema.String,
  scope: Schema.String,
  subscriber_id: Schema.String,
  expires_at: Schema.String,
});

export const ProcessUnsubscribeResponseSchema = Schema.Struct({
  status: Schema.Literal("ok"),
  scope: Schema.String,
});
