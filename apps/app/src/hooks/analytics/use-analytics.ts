import { useCallback, useRef } from "react";
import { usePostHog } from "@/components/providers/posthog-provider.tsx";
import {
  AUTH_EVENTS,
  type AuthEventProperties,
  ONBOARDING_EVENTS,
  type OnboardingEventProperties,
  SUBSCRIPTION_EVENTS,
  type SubscriptionEventProperties,
} from "@/hooks/analytics/events.ts";
import type { Session } from "@/routes/__root.tsx";

type IdentifyUserOptions = {
  userId: string;
  email: string;
  name?: string | null;
  organizationId?: string | null;
  organizationName?: string | null;
  plan?: string;
  isTrialing?: boolean;
};

/**
 * Central analytics hook for tracking events across the app.
 * Provides typed methods for auth, onboarding, and subscription events.
 */
export const useAnalytics = () => {
  const posthog = usePostHog();
  const identifiedRef = useRef<string | null>(null);

  /**
   * Identify user with PostHog and set organization group
   */
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

      // Avoid re-identifying same user
      if (identifiedRef.current === userId) {
        return;
      }

      // Identify the user
      posthog.identify(userId, {
        email,
        name: name || undefined,
        plan: plan || "none",
        is_trialing: isTrialing,
        organization_id: organizationId || undefined,
      });

      // Set organization group for B2B analytics
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

  /**
   * Identify user from session object
   */
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

  /**
   * Reset user identification (on logout)
   */
  const resetUser = useCallback(() => {
    if (!posthog) {
      return;
    }
    posthog.reset();
    identifiedRef.current = null;
  }, [posthog]);

  /**
   * Track auth events with type-safe properties
   */
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

  /**
   * Track onboarding events with type-safe properties
   */
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

  /**
   * Track subscription events with type-safe properties
   */
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

  return {
    identifyUser,
    identifyFromSession,
    resetUser,
    trackAuth,
    trackOnboarding,
    trackSubscription,
    posthog,
  };
};
