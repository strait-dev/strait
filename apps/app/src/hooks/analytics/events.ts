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
  TRIAL_STARTED: "subscription_trial_started",
  PLAN_CHANGED: "subscription_plan_changed",
  CANCELLED: "subscription_cancelled",
  BILLING_INTERVAL_CHANGED: "subscription_billing_interval_changed",
  UPGRADE_PAGE_VIEWED: "subscription_upgrade_page_viewed",
  PORTAL_OPENED: "subscription_portal_opened",
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
