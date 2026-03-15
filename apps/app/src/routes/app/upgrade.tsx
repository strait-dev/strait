import { HugeiconsIcon } from "@hugeicons/react";
import { Alert, AlertDescription } from "@strait/ui/components/alert";
import { Button } from "@strait/ui/components/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@strait/ui/components/card";
import { Shell } from "@strait/ui/components/shell";
import { toast } from "@strait/ui/components/toast/index";
import { useMutation, useSuspenseQuery } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";
import { createServerFn } from "@tanstack/react-start";
import { zodValidator } from "@tanstack/zod-adapter";
import { useCallback, useEffect, useRef, useState } from "react";
import * as z from "zod";
import { PlanSelection } from "@/components/upgrade/plan-selection";
import { useAnalytics } from "@/hooks/analytics/use-analytics";
import { subscriptionStateQueryOptions } from "@/hooks/subscription/use-subscription";
import { AlertCircleIcon, LinkSquareIcon } from "@/lib/icons";
import { getCustomerPortalUrlServerFn } from "@/lib/subscription";
import { authMiddleware } from "@/middlewares/auth";

const PLAN_SLUGS: Record<string, string> = {
  "starter-monthly": "starter-monthly",
  "starter-yearly": "starter-yearly",
  "growth-monthly": "growth-monthly",
  "growth-yearly": "growth-yearly",
  "professional-monthly": "professional-monthly",
  "professional-yearly": "professional-yearly",
  "enterprise-monthly": "enterprise-monthly",
  "enterprise-yearly": "enterprise-yearly",
};

type StartCheckoutInput = {
  planSlug: "starter" | "growth" | "professional" | "enterprise";
  billingInterval: "monthly" | "yearly";
};

const startCheckoutInputSchema = z.object({
  planSlug: z.enum(["starter", "growth", "professional", "enterprise"]),
  billingInterval: z.enum(["monthly", "yearly"]),
});

/**
 * Server function to start checkout for plan upgrade.
 * Creates a Better Auth Polar checkout URL for the selected plan.
 */
const startCheckoutServerFn = createServerFn({ method: "POST" })
  .inputValidator((data: StartCheckoutInput) =>
    startCheckoutInputSchema.parse(data)
  )
  .middleware([authMiddleware])
  .handler(({ data }) => {
    const productSlug = `${data.planSlug}-${data.billingInterval}`;
    const checkoutProductSlug = PLAN_SLUGS[productSlug];

    if (!checkoutProductSlug) {
      throw new Error(`Invalid plan: ${productSlug}`);
    }

    const authBaseUrl =
      process.env.BETTER_AUTH_URL ??
      process.env.VITE_BASE_URL ??
      "http://localhost:5173";

    return {
      checkoutUrl: `${authBaseUrl}/api/auth/checkout/${checkoutProductSlug}`,
    };
  });

const upgradeSearchSchema = z.object({
  canceled: z.string().optional(),
  error: z.string().optional(),
});

export const Route = createFileRoute("/app/upgrade")({
  validateSearch: zodValidator(upgradeSearchSchema),
  component: RouteComponent,
});

function RouteComponent() {
  const search = Route.useSearch();
  const [selectedPlan, setSelectedPlan] = useState<
    "starter" | "growth" | "professional" | "enterprise"
  >("growth");
  const [billingInterval, setBillingInterval] = useState<"monthly" | "yearly">(
    "monthly"
  );
  const { data: subscriptionState } = useSuspenseQuery(
    subscriptionStateQueryOptions()
  );
  const { isActive, isTrialing } = subscriptionState;
  const currentPlan = subscriptionState.plan as
    | "starter"
    | "growth"
    | "professional"
    | "enterprise";
  const [isPortalLoading, setIsPortalLoading] = useState(false);
  const { trackSubscription } = useAnalytics();
  const hasTrackedPageView = useRef(false);

  // Track page view on mount
  useEffect(() => {
    if (!hasTrackedPageView.current) {
      trackSubscription("UPGRADE_PAGE_VIEWED", {
        current_plan: currentPlan,
        is_trialing: isTrialing,
      });
      hasTrackedPageView.current = true;
    }
  }, [trackSubscription, currentPlan, isTrialing]);

  // Pre-select user's current plan on mount (so they upgrade to the plan they're trialing)
  useEffect(() => {
    if (currentPlan) {
      setSelectedPlan(currentPlan);
    }
  }, [currentPlan]);

  const startCheckout = useMutation({
    mutationFn: () =>
      startCheckoutServerFn({
        data: {
          planSlug: selectedPlan,
          billingInterval,
        },
      }),
    onSuccess: (data) => {
      if (data.checkoutUrl) {
        window.location.assign(data.checkoutUrl);
      }
    },
    onError: (error) => {
      toast.error(
        error instanceof Error ? error.message : "Failed to start checkout"
      );
    },
  });

  const handleStartCheckout = useCallback(() => {
    trackSubscription("CHECKOUT_STARTED", {
      plan: selectedPlan,
      billing_interval: billingInterval,
    });
    startCheckout.mutate();
  }, [startCheckout, trackSubscription, selectedPlan, billingInterval]);

  const handleOpenPortal = useCallback(async () => {
    trackSubscription("PORTAL_OPENED");
    setIsPortalLoading(true);
    try {
      const result = await getCustomerPortalUrlServerFn();
      if (result.error || !result.url) {
        toast.error(result.error || "Failed to open customer portal");
        return;
      }
      window.location.href = result.url;
    } catch {
      toast.error("Failed to open customer portal");
    } finally {
      setIsPortalLoading(false);
    }
  }, [trackSubscription]);

  const hasActiveSubscription = isActive;

  return (
    <Shell>
      {/* Show cancellation message if user canceled checkout */}
      {search.canceled ? (
        <Alert className="mb-6 border-yellow-200 bg-yellow-50">
          <HugeiconsIcon
            className="h-4 w-4 text-yellow-600"
            icon={AlertCircleIcon}
          />
          <AlertDescription className="text-yellow-800">
            Checkout was canceled. You can try again by selecting a plan below.
          </AlertDescription>
        </Alert>
      ) : null}

      {/* Show error message if there was an error */}
      {search.error ? (
        <Alert className="mb-6 border-red-200 bg-red-50">
          <HugeiconsIcon
            className="h-4 w-4 text-red-600"
            icon={AlertCircleIcon}
          />
          <AlertDescription className="text-red-800">
            {search.error}
          </AlertDescription>
        </Alert>
      ) : null}

      <div className="space-y-8">
        {/* Portal Access for Existing Customers */}
        {hasActiveSubscription ? (
          <Card className="border-primary/20 bg-primary/5">
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <HugeiconsIcon className="h-5 w-5" icon={LinkSquareIcon} />
                Customer Portal
              </CardTitle>
              <CardDescription>
                Manage your subscription, payment methods and invoices
              </CardDescription>
            </CardHeader>
            <CardContent>
              <Button
                className="flex items-center gap-2"
                disabled={isPortalLoading}
                onClick={handleOpenPortal}
              >
                <HugeiconsIcon className="size-4" icon={LinkSquareIcon} />
                {isPortalLoading ? "Opening..." : "Access Customer Portal"}
              </Button>
            </CardContent>
          </Card>
        ) : null}

        {/* Plan Selection */}
        <PlanSelection
          billingInterval={billingInterval}
          currentPlanSlug={currentPlan}
          isLoading={startCheckout.isPending}
          mode="trial_ended"
          onBillingIntervalChange={setBillingInterval}
          onPlanChange={setSelectedPlan}
          onStartCheckout={handleStartCheckout}
          selectedPlan={selectedPlan}
        />
      </div>
    </Shell>
  );
}
