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
import DowngradePreviewDialog from "@/components/billing/downgrade-preview-dialog";
import ErrorComponent from "@/components/common/error-component";
import type {
  BillingInterval,
  PlanType,
} from "@/components/upgrade/plan-selection";
import { PlanSelection } from "@/components/upgrade/plan-selection";
import { useAnalytics } from "@/hooks/analytics/use-analytics";
import {
  apiPlansToComparisonFeatures,
  apiPlansToPricingPlans,
  getPlansServerFn,
} from "@/hooks/billing/use-plans";
import { subscriptionStateQueryOptions } from "@/hooks/subscription/use-subscription";
import { AlertCircleIcon, LinkSquareIcon } from "@/lib/icons";
import { isDowngrade as checkIsDowngrade } from "@/lib/plan-tiers";
import { getCustomerPortalUrlServerFn } from "@/lib/subscription";
import { authMiddleware } from "@/middlewares/auth";
import type { AppRouteContext } from "@/routes/app/layout";

const PLAN_SLUGS: Record<string, string> = {
  "starter-monthly": "starter-monthly",
  "starter-yearly": "starter-yearly",
  "pro-monthly": "pro-monthly",
  "pro-yearly": "pro-yearly",
  "enterprise-monthly": "enterprise-monthly",
  "enterprise-yearly": "enterprise-yearly",
};

type StartCheckoutInput = {
  planSlug: "starter" | "pro" | "enterprise";
  billingInterval: "monthly" | "yearly";
};

const startCheckoutInputSchema = z.object({
  planSlug: z.enum(["starter", "pro", "enterprise"]),
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
  loader: async ({ context }) => {
    const ctx = context as AppRouteContext;
    const [, apiPlans] = await Promise.all([
      ctx.queryClient.ensureQueryData(subscriptionStateQueryOptions()),
      getPlansServerFn(),
    ]);
    return {
      pricingPlans: apiPlansToPricingPlans(apiPlans),
      comparisonFeatures: apiPlansToComparisonFeatures(apiPlans),
    };
  },
  errorComponent: ErrorComponent,
  component: RouteComponent,
});

function RouteComponent() {
  const search = Route.useSearch();
  const { pricingPlans, comparisonFeatures } = Route.useLoaderData();
  const { data: subscriptionState } = useSuspenseQuery(
    subscriptionStateQueryOptions()
  );
  const { isActive, isTrialing } = subscriptionState;
  const currentPlan = subscriptionState.plan as
    | "free"
    | "starter"
    | "pro"
    | "enterprise";
  const [selectedPlan, setSelectedPlan] = useState<PlanType>(
    currentPlan || "starter"
  );
  const [billingInterval, setBillingInterval] =
    useState<BillingInterval>("monthly");
  const [isPortalLoading, setIsPortalLoading] = useState(false);
  const [downgradeTarget, setDowngradeTarget] = useState<string | null>(null);
  const { trackSubscription } = useAnalytics();
  const hasTrackedPageView = useRef(false);

  useEffect(() => {
    if (!hasTrackedPageView.current) {
      trackSubscription("UPGRADE_PAGE_VIEWED", {
        current_plan: currentPlan,
        is_trialing: isTrialing,
      });
      hasTrackedPageView.current = true;
    }
  }, [trackSubscription, currentPlan, isTrialing]);

  const startCheckout = useMutation({
    mutationFn: () => {
      if (selectedPlan === "free") {
        return Promise.resolve({ checkoutUrl: "/app" });
      }
      return startCheckoutServerFn({
        data: {
          planSlug: selectedPlan as "starter" | "pro" | "enterprise",
          billingInterval,
        },
      });
    },
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

  const isDowngrade = checkIsDowngrade(currentPlan, selectedPlan);

  const handleStartCheckout = useCallback(() => {
    if (isDowngrade) {
      setDowngradeTarget(selectedPlan);
      return;
    }
    trackSubscription("CHECKOUT_STARTED", {
      plan: selectedPlan,
      billing_interval: billingInterval,
    });
    startCheckout.mutate();
  }, [
    startCheckout,
    trackSubscription,
    selectedPlan,
    billingInterval,
    isDowngrade,
  ]);

  const handleConfirmDowngrade = useCallback(() => {
    setDowngradeTarget(null);
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
      {search.canceled ? (
        <Alert className="mb-6 border-yellow-200 bg-yellow-50">
          <HugeiconsIcon
            className="size-4 text-yellow-600"
            icon={AlertCircleIcon}
          />
          <AlertDescription className="text-yellow-800">
            Checkout was canceled. You can try again by selecting a plan below.
          </AlertDescription>
        </Alert>
      ) : null}

      {search.error ? (
        <Alert className="mb-6 border-red-200 bg-red-50">
          <HugeiconsIcon
            className="size-4 text-red-600"
            icon={AlertCircleIcon}
          />
          <AlertDescription className="text-red-800">
            {search.error}
          </AlertDescription>
        </Alert>
      ) : null}

      <div className="space-y-8">
        {hasActiveSubscription ? (
          <Card className="border-border bg-muted/50">
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <HugeiconsIcon className="size-5" icon={LinkSquareIcon} />
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

        <PlanSelection
          billingInterval={billingInterval}
          comparisonFeatures={comparisonFeatures}
          currentPlanSlug={currentPlan}
          isLoading={startCheckout.isPending}
          mode="upgrade"
          onBillingIntervalChange={setBillingInterval}
          onPlanChange={setSelectedPlan}
          onStartCheckout={handleStartCheckout}
          plans={pricingPlans}
          selectedPlan={selectedPlan}
        />
      </div>

      <DowngradePreviewDialog
        isLoading={startCheckout.isPending}
        onConfirm={handleConfirmDowngrade}
        onOpenChange={(open) => {
          if (!open) {
            setDowngradeTarget(null);
          }
        }}
        open={!!downgradeTarget}
        targetTier={downgradeTarget ?? ""}
      />
    </Shell>
  );
}
