import { HugeiconsIcon } from "@hugeicons/react";
import { PLAN_LOOKUP_KEYS } from "@strait/billing";
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
import { toast } from "@strait/ui/components/toast";
import {
  useMutation,
  useQueryClient,
  useSuspenseQuery,
} from "@tanstack/react-query";
import { createFileRoute, redirect } from "@tanstack/react-router";
import { createServerFn } from "@tanstack/react-start";
import { zodValidator } from "@tanstack/zod-adapter";
import { useEffect, useRef, useState } from "react";
import * as z from "zod";
import DowngradePreviewDialog from "@/components/billing/downgrade-preview-dialog";
import ErrorComponent from "@/components/common/error-component";
import type {
  BillingInterval,
  PlanType,
} from "@/components/upgrade/plan-selection";
import { PlanSelection } from "@/components/upgrade/plan-selection";
import { useAnalytics } from "@/hooks/analytics/use-analytics";
import { usePageEvent } from "@/hooks/analytics/use-page-event";
import { orgUsageQueryOptions } from "@/hooks/billing/use-org-usage";
import {
  apiPlansToComparisonFeatures,
  apiPlansToPricingPlans,
  getPlansServerFn,
} from "@/hooks/billing/use-plans";
import { subscriptionStateQueryOptions } from "@/hooks/subscription/use-subscription";
import { getPostHog } from "@/lib/analytics";
import { assertCloudEdition, isCommunityEdition } from "@/lib/edition";
import { AlertCircleIcon, LinkSquareIcon } from "@/lib/icons";
import { isDowngrade as checkIsDowngrade } from "@/lib/plan-tiers";
import { enforceRateLimit } from "@/lib/rate-limit.server";
import {
  findOrCreateCustomerForOrg,
  getStripeClient,
} from "@/lib/stripe.server";
import { getCustomerPortalUrlServerFn } from "@/lib/subscription";
import { authMiddleware } from "@/middlewares/auth";
import { requireActiveOrgAdmin } from "@/middlewares/require-access";
import type { AppRouteContext } from "@/routes/app/layout";

type StartCheckoutInput = {
  planSlug: "starter" | "pro" | "scale" | "business" | "enterprise";
  billingInterval: "monthly" | "yearly";
};

const startCheckoutInputSchema = z.object({
  planSlug: z.enum(["starter", "pro", "scale", "business", "enterprise"]),
  billingInterval: z.enum(["monthly", "yearly"]),
});

/**
 * Server function to start checkout for plan upgrade.
 * Creates a Stripe Checkout Session and returns the URL.
 * Reuses existing Stripe customers to avoid duplicates.
 */
const startCheckoutServerFn = createServerFn({ method: "POST" })
  .middleware([authMiddleware])
  .inputValidator((data: StartCheckoutInput) =>
    startCheckoutInputSchema.parse(data)
  )
  .handler(async ({ data, context }) => {
    // Defense in depth: refuse to talk to Stripe in community edition
    // even if a caller bypasses the route guard below.
    assertCloudEdition("Plan checkout");
    const stripe = getStripeClient();
    const orgId = await requireActiveOrgAdmin(context);

    const slug = `${data.planSlug}-${data.billingInterval}`;
    const lookupKey =
      data.planSlug === "enterprise"
        ? ""
        : PLAN_LOOKUP_KEYS[data.planSlug][
            data.billingInterval === "yearly" ? "annual" : "monthly"
          ];

    if (!lookupKey) {
      throw new Error(`Invalid plan: ${slug}`);
    }

    const prices = await stripe.prices.list({
      active: true,
      limit: 1,
      lookup_keys: [lookupKey],
    });
    const priceId = prices.data[0]?.id;
    if (!priceId) {
      throw new Error(`Missing Stripe price for plan: ${slug}`);
    }

    const baseUrl =
      process.env.BETTER_AUTH_URL ??
      process.env.VITE_BASE_URL ??
      "http://localhost:5173";

    const email = context.user.email;
    if (!email) {
      throw new Error("Email is required for checkout");
    }

    await enforceRateLimit({
      key: `checkout:${orgId}:${context.user.id}`,
      limit: 5,
      windowSeconds: 300,
    });

    const customerId = await findOrCreateCustomerForOrg({
      email,
      orgId,
      userId: context.user.id,
      name: context.user.name,
    });

    const session = await stripe.checkout.sessions.create({
      mode: "subscription",
      line_items: [{ price: priceId, quantity: 1 }],
      success_url: `${baseUrl}/app?checkout_success=true`,
      cancel_url: `${baseUrl}/app/upgrade?canceled=true`,
      customer: customerId,
      allow_promotion_codes: false,
      automatic_tax: { enabled: true },
      subscription_data: {
        metadata: orgId ? { org_id: orgId } : {},
      },
    });

    return {
      checkoutUrl:
        session.url ?? `${baseUrl}/app/upgrade?error=checkout_failed`,
    };
  });

const upgradeSearchSchema = z.object({
  canceled: z.string().optional(),
  error: z.string().optional(),
});

export const Route = createFileRoute("/app/upgrade")({
  validateSearch: zodValidator(upgradeSearchSchema),
  // Cloud-only: plan selection + Stripe checkout are not available
  // in the community edition. Redirect any inbound request to /app.
  beforeLoad: () => {
    if (isCommunityEdition) {
      throw redirect({ to: "/app" });
    }
  },
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
  head: () => ({ meta: [{ title: "Upgrade · Strait" }] }),
  errorComponent: ErrorComponent,
  component: RouteComponent,
});

function RouteComponent() {
  usePageEvent("upgrade_page_viewed");
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
    | "scale"
    | "business"
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
  const queryClient = useQueryClient();

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
    mutationFn: (params: {
      planSlug: "starter" | "pro" | "scale" | "business";
      billingInterval: BillingInterval;
    }) =>
      startCheckoutServerFn({
        data: {
          planSlug: params.planSlug,
          billingInterval: params.billingInterval,
        },
      }),
    onSuccess: async (data, variables) => {
      await Promise.all([
        queryClient.invalidateQueries({
          queryKey: subscriptionStateQueryOptions().queryKey,
        }),
        queryClient.invalidateQueries({
          queryKey: orgUsageQueryOptions().queryKey,
        }),
      ]);
      getPostHog()?.capture("subscription_checkout_started", {
        plan: variables.planSlug,
        billing_interval: variables.billingInterval,
        $set: { plan: variables.planSlug },
      });
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

  const handleStartCheckout = (targetPlan: PlanType = selectedPlan) => {
    if (targetPlan === "free") {
      window.location.assign("/app");
      return;
    }
    if (targetPlan === "enterprise") {
      window.location.assign("/app/enterprise-contact");
      return;
    }
    if (checkIsDowngrade(currentPlan, targetPlan)) {
      setDowngradeTarget(targetPlan);
      return;
    }
    trackSubscription("CHECKOUT_STARTED", {
      plan: targetPlan,
      billing_interval: billingInterval,
    });
    startCheckout.mutate({ planSlug: targetPlan, billingInterval });
  };

  const handleConfirmDowngrade = () => {
    const targetPlan = downgradeTarget as Exclude<PlanType, "free"> | null;
    setDowngradeTarget(null);
    if (!targetPlan || targetPlan === "enterprise") {
      return;
    }
    trackSubscription("CHECKOUT_STARTED", {
      plan: targetPlan,
      billing_interval: billingInterval,
    });
    startCheckout.mutate({ planSlug: targetPlan, billingInterval });
  };

  const handleOpenPortal = async () => {
    trackSubscription("PORTAL_OPENED");
    setIsPortalLoading(true);
    try {
      const result = await getCustomerPortalUrlServerFn();
      if (result.error || !result.url) {
        toast.error(result.error || "Failed to open customer portal");
        setIsPortalLoading(false);
        return;
      }
      window.location.assign(result.url);
    } catch {
      toast.error("Failed to open customer portal");
    }
    setIsPortalLoading(false);
  };

  const hasActiveSubscription = isActive;

  return (
    <Shell>
      <h1 className="sr-only">Upgrade plan</h1>
      {search.canceled ? (
        <Alert className="mb-6" variant="warning">
          <HugeiconsIcon className="size-4" icon={AlertCircleIcon} />
          <AlertDescription>
            Checkout was canceled. You can try again by selecting a plan below.
          </AlertDescription>
        </Alert>
      ) : null}

      {search.error ? (
        <Alert variant="destructive">
          <HugeiconsIcon className="size-4" icon={AlertCircleIcon} />
          <AlertDescription>{search.error}</AlertDescription>
        </Alert>
      ) : null}

      <div className="space-y-8">
        {hasActiveSubscription ? (
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2">
                <HugeiconsIcon className="size-5" icon={LinkSquareIcon} />
                Customer portal
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
                {isPortalLoading ? "Opening..." : "Access customer portal"}
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
