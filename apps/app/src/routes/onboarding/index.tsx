import {
  ArrowLeft01Icon,
  ArrowRight01Icon,
  Loading03Icon,
} from "@hugeicons/core-free-icons";
import { HugeiconsIcon } from "@hugeicons/react";
import { Button } from "@strait/ui/components/button";
import { Progress } from "@strait/ui/components/progress";
import { toast } from "@strait/ui/components/toast/index";
import { useForm } from "@tanstack/react-form";
import { createFileRoute, redirect } from "@tanstack/react-router";
import { createServerFn } from "@tanstack/react-start";
import { getRequestHeaders } from "@tanstack/react-start/server";
import { useCallback, useEffect, useMemo, useRef } from "react";
import { useHotkeys } from "react-hotkeys-hook";
import { BusinessNeedsStep } from "@/components/onboarding/steps/business-needs-step";
import { CompanyInfoStep } from "@/components/onboarding/steps/company-info-step";
import type { OnboardingStepProps } from "@/components/onboarding/types";
import { useOnboardingAnalytics } from "@/hooks/analytics/use-onboarding-analytics";
import { useCompleteOnboarding } from "@/hooks/onboarding/use-onboarding";
import { auth } from "@/lib/auth.server";
import type { OnboardingFormData } from "@/lib/schema";
import { captureException } from "@/lib/sentry";
import type { AuthUser } from "@/routes/__root";
import { useOnboardingStore } from "@/stores/onboarding";
import { PERCENTAGE_MULTIPLIER } from "@/utils/constants";

const getAuthUserFn = createServerFn({ method: "GET" }).handler(async () => {
  try {
    const headers = getRequestHeaders();
    const session = await auth.api.getSession({ headers });
    return (session?.user as AuthUser) ?? null;
  } catch {
    return null;
  }
});

const FINAL_STEP = 2;

/**
 * Validates and tracks the business needs step (Step 1)
 */
function validateBusinessNeedsStep(
  formValues: OnboardingFormData,
  analytics: ReturnType<typeof useOnboardingAnalytics>
): boolean {
  const businessNeeds = formValues.businessNeeds;
  const isValid = businessNeeds && businessNeeds.length >= 1;
  if (isValid) {
    analytics.trackBusinessNeedsCompleted(businessNeeds);
  }
  return isValid;
}

/**
 * Validates and tracks the company info step (Step 2)
 */
function validateCompanyInfoStep(
  formValues: OnboardingFormData,
  analytics: ReturnType<typeof useOnboardingAnalytics>
): boolean {
  const isValid = !!(
    formValues.companyName &&
    formValues.companyName.length >= 2 &&
    formValues.industry &&
    formValues.companySize &&
    formValues.businessType &&
    formValues.country
  );
  if (isValid) {
    analytics.trackCompanyInfoCompleted({
      organizationName: formValues.companyName,
      organizationCountry: formValues.country,
      numberOfEmployees: formValues.companySize,
    });
  }
  return isValid;
}

export const Route = createFileRoute("/onboarding/")({
  beforeLoad: async ({ context }) => {
    if (!context.isAuthenticated) {
      throw redirect({ to: "/login" });
    }

    const authUser = await getAuthUserFn();

    if (authUser?.onboarded === true) {
      throw redirect({ to: "/app" });
    }

    return {};
  },
  component: OnboardingPage,
});

function OnboardingPage() {
  return <OnboardingFlow />;
}

function OnboardingFlow() {
  const { currentStep, totalSteps, setCurrentStep, reset } =
    useOnboardingStore();
  const completeOnboarding = useCompleteOnboarding();
  const analytics = useOnboardingAnalytics();
  const previousStepRef = useRef<number | null>(null);

  const defaultValues = useMemo(
    (): OnboardingFormData => ({
      businessNeeds: [],
      companyName: "",
      companyPhone: "",
      industry: "",
      companySize: "",
      businessType: "",
      annualRevenue: undefined,
      country: "United States",
      website: "",
      primaryGoals: "",
    }),
    []
  );

  const form = useForm({
    defaultValues,
  });

  const progressPercentage = (currentStep / totalSteps) * PERCENTAGE_MULTIPLIER;

  const isBackButtonDisabled = currentStep === 1;

  const getIsStepValid = useCallback(() => {
    const values = form.state.values;
    if (currentStep === 1) {
      return values.businessNeeds && values.businessNeeds.length >= 1;
    }
    if (currentStep === 2) {
      return !!(
        values.companyName &&
        values.companyName.length >= 2 &&
        values.industry &&
        values.companySize &&
        values.businessType &&
        values.country
      );
    }
    return true;
  }, [currentStep, form]);

  const continueButtonText = useMemo(() => {
    if (completeOnboarding.isPending) {
      return (
        <>
          <HugeiconsIcon
            className="h-4 w-4 animate-spin"
            icon={Loading03Icon}
          />
          <span>Creating your store...</span>
        </>
      );
    }
    if (currentStep === FINAL_STEP) {
      return "Continue";
    }
    return (
      <>
        Continue
        <HugeiconsIcon className="h-4 w-4" icon={ArrowRight01Icon} />
      </>
    );
  }, [currentStep, completeOnboarding.isPending]);

  // Track onboarding start and reset state (runs once on mount)
  // biome-ignore lint/correctness/useExhaustiveDependencies: intentionally empty deps - should only run once on mount
  useEffect(() => {
    reset();
    analytics.trackOnboardingStarted();
    analytics.trackBusinessNeedsViewed();
    previousStepRef.current = 1;
  }, []);

  // Track step changes
  useEffect(() => {
    if (
      previousStepRef.current !== null &&
      previousStepRef.current !== currentStep
    ) {
      // Track the new step viewed
      if (currentStep === 1) {
        analytics.trackBusinessNeedsViewed();
      } else if (currentStep === 2) {
        analytics.trackCompanyInfoViewed();
      }
      previousStepRef.current = currentStep;
    }
  }, [currentStep, analytics]);

  const handleBack = useCallback(() => {
    if (currentStep > 1) {
      analytics.trackBackClicked(currentStep);
      setCurrentStep((prev) => prev - 1);
    }
  }, [currentStep, setCurrentStep, analytics]);

  const validateCurrentStep = useCallback((): boolean => {
    const values = form.state.values;
    if (currentStep === 1) {
      return validateBusinessNeedsStep(values, analytics);
    }
    if (currentStep === 2) {
      return validateCompanyInfoStep(values, analytics);
    }
    return false;
  }, [currentStep, form, analytics]);

  const handleOnboardingCompletion = useCallback(() => {
    const formData = form.state.values;

    analytics.trackOnboardingCompleted({
      businessNeeds: formData.businessNeeds,
      organizationName: formData.companyName,
      organizationCountry: formData.country,
    });

    toast.promise(completeOnboarding.mutateAsync(formData), {
      loading: "Creating your store...",
      success: "Store created! Let's choose your plan.",
      error: (error: unknown) => {
        captureException(error);
        analytics.trackOnboardingError(
          error instanceof Error ? error.message : "Unknown error",
          currentStep
        );
        return "Failed to create store. Please try again.";
      },
    });
  }, [form, analytics, completeOnboarding, currentStep]);

  const handleContinue = useCallback(() => {
    const isValid = validateCurrentStep();

    if (!isValid) {
      return;
    }

    if (currentStep === FINAL_STEP) {
      handleOnboardingCompletion();
    } else {
      setCurrentStep((prev) => prev + 1);
    }
  }, [
    currentStep,
    validateCurrentStep,
    handleOnboardingCompletion,
    setCurrentStep,
  ]);

  const handleFormSubmit = useCallback(
    (e: React.FormEvent) => {
      e.preventDefault();
      handleContinue();
    },
    [handleContinue]
  );

  // Keyboard shortcuts for navigation
  useHotkeys(
    "mod+enter",
    () => {
      if (getIsStepValid() && !completeOnboarding.isPending) {
        handleContinue();
      }
    },
    { enableOnFormTags: true },
    [getIsStepValid, completeOnboarding.isPending, handleContinue]
  );

  useHotkeys(
    "mod+backspace",
    () => {
      if (currentStep > 1) {
        handleBack();
      }
    },
    { enableOnFormTags: true },
    [currentStep, handleBack]
  );

  const stepContent = useMemo(() => {
    if (currentStep === 1) {
      return <BusinessNeedsStep form={form as OnboardingStepProps["form"]} />;
    }

    if (currentStep === 2) {
      return (
        <CompanyInfoStep
          form={form as OnboardingStepProps["form"]}
          isLoading={completeOnboarding.isPending}
        />
      );
    }

    return null;
  }, [currentStep, form, completeOnboarding.isPending]);

  return (
    <div className="flex min-h-screen flex-col bg-background">
      {/* Fixed Header */}
      <header className="fixed top-0 right-0 left-0 z-30 border-border border-b bg-background">
        <div className="relative flex h-16 items-center justify-center px-4">
          <Button
            aria-label="Go back to previous step"
            className={`absolute left-4 ${isBackButtonDisabled ? "invisible" : ""}`}
            disabled={isBackButtonDisabled}
            onClick={handleBack}
            size="icon"
            variant="ghost"
          >
            <HugeiconsIcon className="size-5" icon={ArrowLeft01Icon} />
          </Button>

          <img
            alt="Strait Logo"
            className="h-8 w-auto"
            height={32}
            loading="eager"
            src="/strait.svg"
            width={120}
          />
        </div>

        <Progress
          aria-label={`Step ${currentStep} of ${totalSteps}`}
          className="h-0.5 rounded-none"
          value={progressPercentage}
        />
      </header>

      {/* Main content */}
      <main className="mt-[66px] mb-20 flex-1 overflow-auto">
        <div className="container mx-auto px-4 py-6">
          <form
            className="mx-auto max-w-xl"
            data-step={currentStep}
            onSubmit={handleFormSubmit}
          >
            {stepContent}
          </form>
        </div>
      </main>

      {/* Fixed Footer */}
      <footer className="fixed right-0 bottom-0 left-0 z-30 border-border border-t bg-background">
        <div className="container mx-auto px-4 py-4">
          <div className="mx-auto flex max-w-xl items-center gap-3">
            {currentStep > 1 ? (
              <Button
                aria-label="Go back to previous step"
                className="gap-2"
                disabled={completeOnboarding.isPending}
                onClick={handleBack}
                variant="outline"
              >
                <HugeiconsIcon className="size-4" icon={ArrowLeft01Icon} />
                Back
                <kbd className="hidden rounded bg-muted px-1.5 py-0.5 font-mono text-muted-foreground text-xs sm:inline-block">
                  ⌘⌫
                </kbd>
              </Button>
            ) : null}
            <form.Subscribe selector={(state) => state.values}>
              {() => (
                <Button
                  aria-label="Continue to next step"
                  className="flex-1 gap-2"
                  disabled={!getIsStepValid() || completeOnboarding.isPending}
                  onClick={handleContinue}
                >
                  {continueButtonText}
                  {completeOnboarding.isPending ? null : (
                    <kbd className="hidden rounded bg-primary-foreground/20 px-1.5 py-0.5 font-mono text-primary-foreground text-xs sm:inline-block">
                      ⌘↵
                    </kbd>
                  )}
                </Button>
              )}
            </form.Subscribe>
          </div>
        </div>
      </footer>
    </div>
  );
}
