export const AUTH_EVENTS = {
  SIGNED_IN: "auth_signed_in",
  SIGNED_UP: "auth_signed_up",
  SIGNED_OUT: "auth_signed_out",
  LOGOUT: "auth_logout",
  PASSWORD_RESET: "auth_password_reset",
  EMAIL_VERIFIED: "auth_email_verified",
} as const;

export const ONBOARDING_EVENTS = {
  STARTED: "onboarding_started",
  BUSINESS_NEEDS_VIEWED: "onboarding_business_needs_viewed",
  BUSINESS_NEEDS_OPTION_SELECTED: "onboarding_business_needs_option_selected",
  BUSINESS_NEEDS_OPTION_DESELECTED:
    "onboarding_business_needs_option_deselected",
  BUSINESS_NEEDS_COMPLETED: "onboarding_business_needs_completed",
  COMPANY_INFO_VIEWED: "onboarding_company_info_viewed",
  COMPANY_INFO_FIELD_FILLED: "onboarding_company_info_field_filled",
  COMPANY_INFO_COMPLETED: "onboarding_company_info_completed",
  BACK_CLICKED: "onboarding_back_clicked",
  COMPLETED: "onboarding_completed",
  ERROR: "onboarding_error",
  VALIDATION_ERROR: "onboarding_validation_error",
} as const;

export const SUBSCRIPTION_EVENTS = {
  CHECKOUT_STARTED: "subscription_checkout_started",
  CHECKOUT_COMPLETED: "subscription_checkout_completed",
  PLAN_CHANGED: "subscription_plan_changed",
  CANCELLED: "subscription_cancelled",
  BILLING_INTERVAL_CHANGED: "subscription_billing_interval_changed",
  UPGRADE_PAGE_VIEWED: "subscription_upgrade_page_viewed",
  PORTAL_OPENED: "subscription_portal_opened",
} as const;

export const PRODUCT_EVENTS = {
  JOB_TRIGGERED: "job_triggered",
  JOB_PAUSED: "job_paused",
  JOB_RESUMED: "job_resumed",
  JOB_DELETED: "job_deleted",
  RUN_RETRIED: "run_retried",
  RUN_CANCELED: "run_canceled",
  SCHEDULE_PAUSED: "schedule_paused",
  SCHEDULE_RESUMED: "schedule_resumed",
  SCHEDULE_TRIGGERED: "schedule_triggered",
  WORKFLOW_TRIGGERED: "workflow_triggered",
  WORKFLOW_PAUSED: "workflow_paused",
  WORKFLOW_RESUMED: "workflow_resumed",
  WEBHOOK_CREATED: "webhook_created",
  WEBHOOK_DELETED: "webhook_deleted",
  WEBHOOK_TESTED: "webhook_tested",
  DLQ_ITEM_RETRIED: "dlq_item_retried",
  DLQ_ITEM_DISCARDED: "dlq_item_discarded",
  DLQ_BULK_RETRIED: "dlq_bulk_retried",
  DLQ_BULK_DISCARDED: "dlq_bulk_discarded",
  API_KEY_CREATED: "api_key_created",
  API_KEY_REVOKED: "api_key_revoked",
  API_KEY_ROTATED: "api_key_rotated",
  MUTATION_ERROR: "mutation_error",
} as const;

export const TEAM_EVENTS = {
  ORG_CREATED: "org_created",
  ORG_UPDATED: "org_updated",
  ORG_DELETION_REQUESTED: "org_deletion_requested",
  MEMBER_INVITED: "member_invited",
  MEMBER_REMOVED: "member_removed",
  MEMBER_ROLE_CHANGED: "member_role_changed",
  INVITATION_ACCEPTED: "invitation_accepted",
  INVITATION_REJECTED: "invitation_rejected",
  PROJECT_CREATED: "project_created",
  PROJECT_DELETED: "project_deleted",
  PROJECT_SWITCHED: "project_switched",
  PROJECT_SETTINGS_UPDATED: "project_settings_updated",
} as const;

export const NAVIGATION_EVENTS = {
  DASHBOARD_VIEWED: "dashboard_viewed",
  JOBS_LIST_VIEWED: "jobs_list_viewed",
  JOB_DETAIL_VIEWED: "job_detail_viewed",
  RUNS_LIST_VIEWED: "runs_list_viewed",
  RUN_DETAIL_VIEWED: "run_detail_viewed",
  SCHEDULES_LIST_VIEWED: "schedules_list_viewed",
  SCHEDULE_DETAIL_VIEWED: "schedule_detail_viewed",
  WORKFLOWS_LIST_VIEWED: "workflows_list_viewed",
  WORKFLOW_DETAIL_VIEWED: "workflow_detail_viewed",
  DLQ_VIEWED: "dlq_viewed",
  LOGS_VIEWED: "logs_viewed",
  EVENTS_VIEWED: "events_viewed",
  WEBHOOKS_VIEWED: "webhooks_viewed",
  SETTINGS_VIEWED: "settings_viewed",
  BILLING_VIEWED: "billing_viewed",
  UPGRADE_PAGE_VIEWED: "upgrade_page_viewed",
  PRICING_COMPARED: "pricing_compared",
  FEEDBACK_SUBMITTED: "feedback_submitted",
  SUPPORT_OPENED: "support_opened",
  SUPPORT_SUBMITTED: "support_submitted",
} as const;

export const BILLING_EVENTS = {
  SPENDING_LIMIT_UPDATED: "spending_limit_updated",
  USAGE_EXPORT_CSV: "usage_export_csv",
  USAGE_EXPORT_PDF: "usage_export_pdf",
  BILLING_PORTAL_OPENED: "billing_portal_opened",
} as const;

export type AuthEventProperties = {
  [K in keyof typeof AUTH_EVENTS]?: Record<string, unknown>;
};

export type OnboardingEventProperties = {
  [K in keyof typeof ONBOARDING_EVENTS]?: Record<string, unknown>;
};

export type SubscriptionEventProperties = {
  [K in keyof typeof SUBSCRIPTION_EVENTS]?: Record<string, unknown>;
};

export type ProductEventProperties = {
  [K in keyof typeof PRODUCT_EVENTS]?: Record<string, unknown>;
};

export type TeamEventProperties = {
  [K in keyof typeof TEAM_EVENTS]?: Record<string, unknown>;
};

export type NavigationEventProperties = {
  [K in keyof typeof NAVIGATION_EVENTS]?: Record<string, unknown>;
};

export type BillingEventProperties = {
  [K in keyof typeof BILLING_EVENTS]?: Record<string, unknown>;
};
