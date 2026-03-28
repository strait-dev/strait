import { useCallback, useRef } from "react";
import { usePostHog } from "@/components/providers/posthog-provider";
import {
  AUTH_EVENTS,
  type AuthEventProperties,
  BILLING_EVENTS,
  type BillingEventProperties,
  NAVIGATION_EVENTS,
  type NavigationEventProperties,
  ONBOARDING_EVENTS,
  type OnboardingEventProperties,
  PRODUCT_EVENTS,
  type ProductEventProperties,
  SUBSCRIPTION_EVENTS,
  type SubscriptionEventProperties,
  TEAM_EVENTS,
  type TeamEventProperties,
} from "@/hooks/analytics/events";
import type { Session } from "@/routes/__root";

type IdentifyUserOptions = {
  userId: string;
  email: string;
  name?: string | null;
  organizationId?: string | null;
  organizationName?: string | null;
  plan?: string;
  isTrialing?: boolean;
};

export const useAnalytics = () => {
  const posthog = usePostHog();
  const identifiedRef = useRef<string | null>(null);

  const identifyUser = useCallback(
    ({
      userId,
      email,
      name,
      organizationId,
      organizationName,
      plan,
      isTrialing,
    }: IdentifyUserOptions) => {
      if (!(posthog && userId)) {
        return;
      }

      if (identifiedRef.current === userId) {
        return;
      }

      posthog.identify(userId, {
        email,
        name: name || undefined,
        plan: plan || "none",
        is_trialing: isTrialing,
        organization_id: organizationId || undefined,
      });

      if (organizationId) {
        posthog.group("organization", organizationId, {
          name: organizationName || undefined,
          plan: plan || "none",
        });
      }

      identifiedRef.current = userId;
    },
    [posthog]
  );

  const identifyFromSession = useCallback(
    (
      session: Session,
      options?: {
        plan?: string;
        isTrialing?: boolean;
        organizationName?: string;
      }
    ) => {
      if (!session?.user?.id) {
        return;
      }

      identifyUser({
        userId: session.user.id,
        email: session.user.email,
        name: session.user.name,
        organizationId: session.user.defaultOrganizationId,
        organizationName: options?.organizationName,
        plan: options?.plan,
        isTrialing: options?.isTrialing,
      });
    },
    [identifyUser]
  );

  const resetUser = useCallback(() => {
    if (!posthog) {
      return;
    }
    posthog.reset();
    identifiedRef.current = null;
  }, [posthog]);

  const trackAuth = useCallback(
    <E extends keyof typeof AUTH_EVENTS>(
      event: E,
      properties?: AuthEventProperties[E]
    ) => {
      if (!posthog) {
        return;
      }
      posthog.capture(AUTH_EVENTS[event], properties);
    },
    [posthog]
  );

  const trackOnboarding = useCallback(
    <E extends keyof typeof ONBOARDING_EVENTS>(
      event: E,
      properties?: OnboardingEventProperties[E]
    ) => {
      if (!posthog) {
        return;
      }
      posthog.capture(ONBOARDING_EVENTS[event], properties);
    },
    [posthog]
  );

  const trackSubscription = useCallback(
    <E extends keyof typeof SUBSCRIPTION_EVENTS>(
      event: E,
      properties?: SubscriptionEventProperties[E]
    ) => {
      if (!posthog) {
        return;
      }
      posthog.capture(SUBSCRIPTION_EVENTS[event], properties);
    },
    [posthog]
  );

  const trackProduct = useCallback(
    <E extends keyof typeof PRODUCT_EVENTS>(
      event: E,
      properties?: ProductEventProperties[E]
    ) => {
      if (!posthog) {
        return;
      }
      posthog.capture(PRODUCT_EVENTS[event], properties);
    },
    [posthog]
  );

  const trackTeam = useCallback(
    <E extends keyof typeof TEAM_EVENTS>(
      event: E,
      properties?: TeamEventProperties[E]
    ) => {
      if (!posthog) {
        return;
      }
      posthog.capture(TEAM_EVENTS[event], properties);
    },
    [posthog]
  );

  const trackNavigation = useCallback(
    <E extends keyof typeof NAVIGATION_EVENTS>(
      event: E,
      properties?: NavigationEventProperties[E]
    ) => {
      if (!posthog) {
        return;
      }
      posthog.capture(NAVIGATION_EVENTS[event], properties);
    },
    [posthog]
  );

  const trackBilling = useCallback(
    <E extends keyof typeof BILLING_EVENTS>(
      event: E,
      properties?: BillingEventProperties[E]
    ) => {
      if (!posthog) {
        return;
      }
      posthog.capture(BILLING_EVENTS[event], properties);
    },
    [posthog]
  );

  return {
    identifyUser,
    identifyFromSession,
    resetUser,
    trackAuth,
    trackOnboarding,
    trackSubscription,
    trackProduct,
    trackTeam,
    trackNavigation,
    trackBilling,
    posthog,
  };
};
