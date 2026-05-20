import { Badge } from "@strait/ui/components/badge";
import { Button } from "@strait/ui/components/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@strait/ui/components/card";
import { toast } from "@strait/ui/components/toast/index";
import { useQuery } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { createServerFn } from "@tanstack/react-start";
import { useState } from "react";
import { ADDON_CATALOG, getActivePackCount } from "@/hooks/billing/use-addons";
import { orgUsageQueryOptions } from "@/hooks/billing/use-org-usage";
import { apiRequest } from "@/lib/api-client.server";
import { assertCloudEdition } from "@/lib/edition";
import { enforceRateLimit } from "@/lib/rate-limit.server";
import {
  findOrCreateCustomerForOrg,
  getStripeClient,
} from "@/lib/stripe.server";
import { authMiddleware } from "@/middlewares/auth";
import { requireActiveOrgAdmin } from "@/middlewares/require-access";

const getAddonPriceMap = (): Record<string, string | undefined> => ({
  "addon-concurrent-runs": process.env.STRIPE_ADDON_CONCURRENT_RUNS_PRICE_ID,
  "addon-members": process.env.STRIPE_ADDON_MEMBERS_PRICE_ID,
  "addon-cron-schedules": process.env.STRIPE_ADDON_CRON_SCHEDULES_PRICE_ID,
  "addon-data-retention": process.env.STRIPE_ADDON_DATA_RETENTION_PRICE_ID,
  "addon-webhook-endpoints":
    process.env.STRIPE_ADDON_WEBHOOK_ENDPOINTS_PRICE_ID,
});

/** Plans that can purchase add-ons. Enterprise has custom terms. */
const ADDON_ELIGIBLE_PLANS = new Set(["starter", "pro", "scale", "business"]);

const startAddonCheckoutServerFn = createServerFn({ method: "POST" })
  .inputValidator((data: { checkoutSlug: string }) => data)
  .middleware([authMiddleware])
  .handler(async ({ data, context }) => {
    // Defense in depth: refuse to talk to Stripe in community edition
    // even though this component is already unreachable from the nav.
    assertCloudEdition("Addon checkout");
    const stripe = getStripeClient();
    const orgId = await requireActiveOrgAdmin(context);

    const priceId = getAddonPriceMap()[data.checkoutSlug];
    if (!priceId) {
      throw new Error(`Invalid addon: ${data.checkoutSlug}`);
    }

    const usage = await apiRequest<{ plan?: string }>("/v1/usage/current", {
      params: { org_id: orgId },
    });
    if (!ADDON_ELIGIBLE_PLANS.has(usage.plan ?? "free")) {
      throw new Error("Add-ons are not available for the current plan");
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
      key: `addon-checkout:${orgId}:${context.user.id}`,
      limit: 10,
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
      success_url: `${baseUrl}/app/billing?addon_success=true`,
      cancel_url: `${baseUrl}/app/billing`,
      customer: customerId,
      allow_promotion_codes: true,
      automatic_tax: { enabled: true },
      subscription_data: {
        metadata: { org_id: orgId },
      },
    });

    return {
      checkoutUrl:
        session.url ?? `${baseUrl}/app/billing?error=checkout_failed`,
    };
  });

const AddonsTab = () => {
  const { data: usage } = useQuery(orgUsageQueryOptions());
  const navigate = useNavigate();
  const [loadingSlug, setLoadingSlug] = useState<string | null>(null);

  const plan = usage?.plan ?? "free";
  const isEligible = ADDON_ELIGIBLE_PLANS.has(plan);
  const activeAddons = usage?.active_addons;

  if (!isEligible) {
    const message =
      plan === "enterprise"
        ? "Enterprise plans have custom limits. Contact your account manager to adjust."
        : "Add-ons are available on paid plans.";

    const action =
      plan === "enterprise" ? null : (
        <Button
          onClick={() => navigate({ to: "/app/upgrade" })}
          variant="default"
        >
          Upgrade to Starter
        </Button>
      );

    return (
      <Card>
        <CardContent className="flex flex-col items-center gap-4 py-12">
          <p className="text-muted-foreground text-sm">{message}</p>
          {action}
        </CardContent>
      </Card>
    );
  }

  return (
    <div className="space-y-4">
      <div>
        <h3 className="font-medium text-foreground text-sm tracking-tight">
          Add-on packs
        </h3>
        <p className="text-muted-foreground text-sm">
          Expand specific limits without upgrading your plan. Each pack is
          billed as a separate monthly subscription.
        </p>
      </div>

      <div className="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-3">
        {ADDON_CATALOG.map((addon) => {
          const activePacks = getActivePackCount(activeAddons, addon.type);

          return (
            <Card key={addon.type}>
              <CardHeader className="pb-2">
                <div className="flex items-center justify-between">
                  <CardTitle className="font-medium text-sm">
                    {addon.name}
                  </CardTitle>
                  {activePacks > 0 && (
                    <Badge variant="secondary">
                      {activePacks} {activePacks === 1 ? "pack" : "packs"}
                    </Badge>
                  )}
                </div>
                <CardDescription className="text-xs">
                  {addon.description}
                </CardDescription>
              </CardHeader>
              <CardContent>
                <div className="flex items-center justify-between">
                  <div>
                    <p className="font-medium text-foreground text-sm">
                      +{addon.packSize} {addon.packUnit}
                    </p>
                    <p className="text-muted-foreground text-xs">
                      {addon.price} per pack
                    </p>
                  </div>
                  <Button
                    disabled={loadingSlug === addon.checkoutSlug}
                    onClick={async () => {
                      setLoadingSlug(addon.checkoutSlug);
                      try {
                        const result = await startAddonCheckoutServerFn({
                          data: { checkoutSlug: addon.checkoutSlug },
                        });
                        if (result.checkoutUrl) {
                          window.location.assign(result.checkoutUrl);
                        }
                      } catch {
                        toast.error("Failed to start addon checkout");
                      } finally {
                        setLoadingSlug(null);
                      }
                    }}
                    variant="outline"
                  >
                    {loadingSlug === addon.checkoutSlug
                      ? "Loading..."
                      : "Add pack"}
                  </Button>
                </div>
              </CardContent>
            </Card>
          );
        })}
      </div>
    </div>
  );
};

export default AddonsTab;
