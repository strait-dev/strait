import {
  ArrowRight02Icon,
  CheckmarkCircle02Icon,
} from "@hugeicons/core-free-icons";
import { HugeiconsIcon } from "@hugeicons/react";
import {
  formatPlanPrice,
  formatPriceWithCents,
  PLAN_KEYS,
  PLANS,
} from "@strait/billing/products";
import { Badge } from "@strait/ui/components/badge";
import { Button } from "@strait/ui/components/button";
import { cn } from "@strait/ui/utils";
import { useMemo, useState } from "react";

import { dashboardHref } from "@/lib/urls.ts";

function planBorderClass(idx: number): string {
  if (idx === 0) {
    return "border-t sm:border-t lg:border-l-0";
  }
  return "border-t sm:border-t-0 sm:border-l lg:border-t-0 lg:border-l";
}

export function StaticPricingTable() {
  const [interval, setInterval] = useState<"monthly" | "yearly">("yearly");

  const savingsPercent = useMemo(() => {
    const monthly = PLANS.starter.prices.monthly * 12;
    const yearly = PLANS.starter.prices.yearly;
    return Math.round(((monthly - yearly) / monthly) * 100);
  }, []);

  return (
    <div className="mt-10 sm:mt-12">
      <div className="flex justify-center px-1">
        <div className="inline-flex items-center gap-1 rounded-full border border-border/60 bg-card p-1">
          <button
            className={cn(
              "min-h-11 rounded-full px-5 py-2.5 font-medium transition-colors",
              interval === "monthly"
                ? "bg-primary text-primary-foreground shadow-sm"
                : "text-muted-foreground hover:text-foreground"
            )}
            onClick={() => setInterval("monthly")}
            type="button"
          >
            Monthly
          </button>
          <button
            className={cn(
              "min-h-11 rounded-full px-5 py-2.5 font-medium transition-colors",
              interval === "yearly"
                ? "bg-primary text-primary-foreground shadow-sm"
                : "text-muted-foreground hover:text-foreground"
            )}
            onClick={() => setInterval("yearly")}
            type="button"
          >
            Yearly
          </button>
          <span className="mr-2 ml-1 rounded-full bg-muted px-3 py-1 font-medium text-foreground">
            Save ~{savingsPercent}%
          </span>
        </div>
      </div>

      <div className="mt-10 overflow-hidden rounded-xl border border-border/60">
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-5">
          {PLAN_KEYS.map((key, idx) => {
            const plan = PLANS[key];
            const isEnterprise = key === "enterprise";
            const isFree = key === "free";
            const priceDisplay = formatPlanPrice(plan, interval);
            const href = isEnterprise
              ? plan.cta.href
              : dashboardHref(plan.cta.href);

            return (
              <div
                className={cn(
                  "relative flex h-full flex-col",
                  plan.highlighted && "bg-primary/[0.04]",
                  planBorderClass(idx)
                )}
                key={key}
              >
                {/* Header — uniform structure for all plans */}
                <div className="px-4 pt-5">
                  <div className="flex items-center gap-2">
                    <h3 className="text-foreground text-lg">{plan.name}</h3>
                    {plan.highlighted && (
                      <Badge
                        className="border-primary/30 text-primary"
                        variant="outline"
                      >
                        Popular
                      </Badge>
                    )}
                    {plan.badge &&
                      !plan.highlighted &&
                      plan.badge !== "Most popular" && (
                        <Badge variant="outline">{plan.badge}</Badge>
                      )}
                  </div>
                  <p className="mt-1.5 whitespace-pre-line text-muted-foreground leading-relaxed">
                    {plan.description}
                  </p>
                </div>

                <div className="flex flex-1 flex-col px-4 pb-5">
                  <div className="mt-5 mb-5">
                    {interval === "yearly" && !(isEnterprise || isFree) ? (
                      <div className="flex items-baseline gap-1.5">
                        <span className="text-3xl text-foreground tabular-nums">
                          {formatPriceWithCents(plan.prices.yearly)}
                        </span>
                        <span className="text-muted-foreground">/yr</span>
                        <span className="text-muted-foreground/50">
                          {priceDisplay}/mo
                        </span>
                      </div>
                    ) : (
                      <div className="flex items-baseline gap-1.5">
                        <span className="text-3xl text-foreground tabular-nums">
                          {priceDisplay}
                        </span>
                        {!(isEnterprise || isFree) && (
                          <span className="text-muted-foreground">/mo</span>
                        )}
                      </div>
                    )}
                  </div>

                  {plan.trial && (
                    <p className="mb-4 text-muted-foreground">
                      14-day free trial included
                    </p>
                  )}

                  <div className="mb-5 border-border/40 border-t" />

                  {plan.computeCredit !== "100 runs/mo (micro, 10s)" && (
                    <p className="mb-3 font-medium text-foreground">
                      {plan.computeCredit} compute credit
                    </p>
                  )}

                  <ul className="mb-6 flex-1 space-y-2">
                    {plan.features.map((feature) => (
                      <li
                        className="flex items-start gap-2 leading-relaxed"
                        key={feature}
                      >
                        <HugeiconsIcon
                          className="mt-0.5 size-4 shrink-0 text-foreground"
                          icon={CheckmarkCircle02Icon}
                        />
                        <span className="text-pretty text-muted-foreground">
                          {feature}
                        </span>
                      </li>
                    ))}
                  </ul>

                  <Button
                    className="w-full transition-shadow duration-150"
                    render={<a href={href} />}
                    variant={plan.highlighted ? "default" : "outline"}
                  >
                    {plan.cta.label}
                    <HugeiconsIcon className="size-4" icon={ArrowRight02Icon} />
                  </Button>
                </div>
              </div>
            );
          })}
        </div>
      </div>

      <p className="mt-8 text-center text-muted-foreground/60">
        All plans include core orchestration capabilities. Cancel anytime.
      </p>
    </div>
  );
}
