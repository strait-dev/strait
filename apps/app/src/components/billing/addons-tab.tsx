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
import { authMiddleware } from "@/middlewares/auth";

const ADDON_PRICE_MAP: Record<string, string | undefined> = {
  "addon-concurrent-runs": process.env.STRIPE_ADDON_CONCURRENT_RUNS_PRICE_ID,
  "addon-members": process.env.STRIPE_ADDON_MEMBERS_PRICE_ID,
  "addon-cron-schedules": process.env.STRIPE_ADDON_CRON_SCHEDULES_PRICE_ID,
  "addon-data-retention": process.env.STRIPE_ADDON_DATA_RETENTION_PRICE_ID,
  "addon-webhook-endpoints":
    process.env.STRIPE_ADDON_WEBHOOK_ENDPOINTS_PRICE_ID,
};

const startAddonCheckoutServerFn = createServerFn({ method: "POST" })
  .inputValidator((data: { checkoutSlug: string }) => data)
  .middleware([authMiddleware])
  .handler(async ({ data, context }) => {
    const { getStripeClient, findOrCreateCustomer } = await import(
      "@/lib/stripe.server"
    );
    const stripe = getStripeClient();

    const priceId = ADDON_PRICE_MAP[data.checkoutSlug];
    if (!priceId) {
      throw new Error(`Invalid addon: ${data.checkoutSlug}`);
    }

    const baseUrl =
      process.env.BETTER_AUTH_URL ??
      process.env.VITE_BASE_URL ??
      "http://localhost:5173";

    const ctx = context as unknown as {
      session?: {
        user: { email: string };
        session: { activeOrganizationId?: string };
      };
    };
    const email = ctx?.session?.user?.email;
    const orgId = ctx?.session?.session?.activeOrganizationId;

    const customerId = email
      ? await findOrCreateCustomer(
          email,
          orgId ? { org_id: orgId } : undefined
        )
      : undefined;

    const session = await stripe.checkout.sessions.create({
      mode: "subscription",
      line_items: [{ price: priceId, quantity: 1 }],
      success_url: `${baseUrl}/app/billing?addon_success=true`,
      cancel_url: `${baseUrl}/app/billing`,
      ...(customerId ? { customer: customerId } : { customer_email: email }),
      allow_promotion_codes: true,
      automatic_tax: { enabled: true },
      subscription_data: {
        metadata: orgId ? { org_id: orgId } : {},
      },
    });

    return {
      checkoutUrl:
        session.url ?? `${baseUrl}/app/billing?error=checkout_failed`,
    };
  });

/** Plans that can purchase add-ons. Enterprise has custom terms. */
const ADDON_ELIGIBLE_PLANS = new Set(["starter", "pro", "scale"]);

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
        <h3 className="font-normal text-base text-foreground tracking-tight">
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
                    size="sm"
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
