import { useCallback, useRef } from "react";
import { usePostHog } from "@/components/providers/posthog-provider.tsx";
import { ONBOARDING_EVENTS } from "@/hooks/analytics/events.ts";

/**
 * Hook for tracking onboarding analytics events.
 * Uses centralized event constants for consistent event naming.
 */
export const useOnboardingAnalytics = () => {
  const posthog = usePostHog();
  const onboardingStartTimeRef = useRef<number | null>(null);

  /**
   * Track when onboarding is started
   */
  const trackOnboardingStarted = useCallback(() => {
    onboardingStartTimeRef.current = Date.now();
    posthog?.capture(ONBOARDING_EVENTS.STARTED, {
      timestamp: new Date().toISOString(),
    });
  }, [posthog]);

  /**
   * Track when business needs step is viewed
   */
  const trackBusinessNeedsViewed = useCallback(() => {
    posthog?.capture(ONBOARDING_EVENTS.BUSINESS_NEEDS_VIEWED, {
      timestamp: new Date().toISOString(),
    });
  }, [posthog]);

  /**
   * Track when a business need option is selected
   */
  const trackBusinessNeedSelected = useCallback(
    (need: string, totalSelected: number) => {
      posthog?.capture(ONBOARDING_EVENTS.BUSINESS_NEEDS_OPTION_SELECTED, {
        need,
        total_selected: totalSelected,
        timestamp: new Date().toISOString(),
      });
    },
    [posthog]
  );

  /**
   * Track when a business need option is deselected
   */
  const trackBusinessNeedDeselected = useCallback(
    (need: string, totalSelected: number) => {
      posthog?.capture(ONBOARDING_EVENTS.BUSINESS_NEEDS_OPTION_DESELECTED, {
        need,
        total_selected: totalSelected,
        timestamp: new Date().toISOString(),
      });
    },
    [posthog]
  );

  /**
   * Track when business needs step is completed
   */
  const trackBusinessNeedsCompleted = useCallback(
    (selectedNeeds: string[]) => {
      posthog?.capture(ONBOARDING_EVENTS.BUSINESS_NEEDS_COMPLETED, {
        selected_needs: selectedNeeds,
        needs_count: selectedNeeds.length,
        timestamp: new Date().toISOString(),
      });
    },
    [posthog]
  );

  /**
   * Track when company info step is viewed
   */
  const trackCompanyInfoViewed = useCallback(() => {
    posthog?.capture(ONBOARDING_EVENTS.COMPANY_INFO_VIEWED, {
      timestamp: new Date().toISOString(),
    });
  }, [posthog]);

  /**
   * Track when a company info field is filled
   */
  const trackCompanyInfoFieldFilled = useCallback(
    (fieldName: string) => {
      posthog?.capture(ONBOARDING_EVENTS.COMPANY_INFO_FIELD_FILLED, {
        field_name: fieldName,
        timestamp: new Date().toISOString(),
      });
    },
    [posthog]
  );

  /**
   * Track when company info step is completed
   */
  const trackCompanyInfoCompleted = useCallback(
    (companyInfo: {
      organizationName: string;
      organizationCountry: string;
      numberOfEmployees?: string;
    }) => {
      posthog?.capture(ONBOARDING_EVENTS.COMPANY_INFO_COMPLETED, {
        organization_name: companyInfo.organizationName,
        organization_country: companyInfo.organizationCountry,
        number_of_employees: companyInfo.numberOfEmployees,
        timestamp: new Date().toISOString(),
      });
    },
    [posthog]
  );

  /**
   * Track when user clicks back button
   */
  const trackBackClicked = useCallback(
    (fromStep: number) => {
      posthog?.capture(ONBOARDING_EVENTS.BACK_CLICKED, {
        from_step: fromStep,
        to_step: fromStep - 1,
        timestamp: new Date().toISOString(),
      });
    },
    [posthog]
  );

  /**
   * Track onboarding completion
   */
  const trackOnboardingCompleted = useCallback(
    (properties: {
      businessNeeds?: string[];
      organizationName?: string;
      organizationCountry?: string;
    }) => {
      const timeOnOnboarding = onboardingStartTimeRef.current
        ? Date.now() - onboardingStartTimeRef.current
        : undefined;

      posthog?.capture(ONBOARDING_EVENTS.COMPLETED, {
        ...properties,
        time_on_onboarding_ms: timeOnOnboarding,
        timestamp: new Date().toISOString(),
      });
    },
    [posthog]
  );

  /**
   * Track onboarding errors
   */
  const trackOnboardingError = useCallback(
    (error: string, step: number) => {
      posthog?.capture(ONBOARDING_EVENTS.ERROR, {
        error,
        step,
        timestamp: new Date().toISOString(),
      });
    },
    [posthog]
  );

  /**
   * Track validation errors
   */
  const trackValidationError = useCallback(
    (field: string, message: string, step: number) => {
      posthog?.capture(ONBOARDING_EVENTS.VALIDATION_ERROR, {
        field,
        message,
        step,
        timestamp: new Date().toISOString(),
      });
    },
    [posthog]
  );

  return {
    trackOnboardingStarted,
    trackBusinessNeedsViewed,
    trackBusinessNeedSelected,
    trackBusinessNeedDeselected,
    trackBusinessNeedsCompleted,
    trackCompanyInfoViewed,
    trackCompanyInfoFieldFilled,
    trackCompanyInfoCompleted,
    trackBackClicked,
    trackOnboardingCompleted,
    trackOnboardingError,
    trackValidationError,
  };
};
