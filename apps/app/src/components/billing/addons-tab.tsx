import { Badge } from "@strait/ui/components/badge";
import { Button } from "@strait/ui/components/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@strait/ui/components/card";
import { useQuery } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { ADDON_CATALOG, getActivePackCount } from "@/hooks/billing/use-addons";
import { orgUsageQueryOptions } from "@/hooks/billing/use-org-usage";

/** Plans that can purchase add-ons. Enterprise has custom terms. */
const ADDON_ELIGIBLE_PLANS = new Set(["starter", "pro", "scale"]);

const AddonsTab = () => {
  const { data: usage } = useQuery(orgUsageQueryOptions());
  const navigate = useNavigate();

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
          Expand specific limits without upgrading your plan. Each pack is billed
          as a separate monthly subscription.
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
                  <a href={`/api/auth/checkout/${addon.checkoutSlug}`}>
                    <Button size="sm" variant="outline">
                      Add pack
                    </Button>
                  </a>
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
