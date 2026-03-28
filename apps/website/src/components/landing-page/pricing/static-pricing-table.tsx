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
              "min-h-11 rounded-full px-5 py-2.5 font-medium text-sm transition-colors",
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
              "min-h-11 rounded-full px-5 py-2.5 font-medium text-sm transition-colors",
              interval === "yearly"
                ? "bg-primary text-primary-foreground shadow-sm"
                : "text-muted-foreground hover:text-foreground"
            )}
            onClick={() => setInterval("yearly")}
            type="button"
          >
            Yearly
          </button>
          <span className="mr-2 ml-1 rounded-full bg-muted px-3 py-1 font-medium text-foreground text-xs">
            Save ~{savingsPercent}%
          </span>
        </div>
      </div>

      <div className="mt-10 grid grid-cols-1 gap-6 sm:grid-cols-2 lg:grid-cols-4 lg:gap-5">
        {PLAN_KEYS.map((key) => {
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
                "relative flex h-full flex-col overflow-hidden rounded-xl border transition-shadow duration-150",
                plan.highlighted
                  ? "border-primary/40"
                  : "border-border/60 bg-card hover:border-border hover:shadow-md"
              )}
              key={key}
            >
              {plan.highlighted ? (
                <div className="relative bg-primary px-5 py-6 sm:px-6">
                  <div className="showcase-dots pointer-events-none absolute inset-0" />
                  <div
                    className="pointer-events-none absolute inset-0 opacity-30"
                    style={{
                      background:
                        "radial-gradient(circle at 30% 20%, oklch(1 0 0 / 0.2), transparent 50%), radial-gradient(circle at 70% 80%, oklch(1 0 0 / 0.1), transparent 50%)",
                    }}
                  />
                  <div className="relative z-10">
                    <span className="mb-3 inline-block rounded-md bg-primary-foreground/20 px-2.5 py-1 font-medium text-primary-foreground text-xs backdrop-blur-sm">
                      Most popular
                    </span>
                    <h3 className="text-primary-foreground text-xl">
                      {plan.name}
                    </h3>
                    <p className="mt-2 max-w-sm text-pretty text-primary-foreground/70 text-sm leading-relaxed">
                      {plan.description}
                    </p>
                  </div>
                </div>
              ) : (
                <div className="px-5 pt-6 sm:px-6">
                  <div className="flex items-center gap-2">
                    <h3 className="text-foreground text-xl">{plan.name}</h3>
                    {plan.badge && plan.badge !== "Most popular" && (
                      <Badge variant="outline">{plan.badge}</Badge>
                    )}
                  </div>
                  <p className="mt-2 max-w-sm text-pretty text-muted-foreground text-sm leading-relaxed">
                    {plan.description}
                  </p>
                </div>
              )}

              <div className="flex flex-1 flex-col px-5 pb-6 sm:px-6">
                <div className="mt-6 mb-6">
                  {interval === "yearly" && !(isEnterprise || isFree) ? (
                    <>
                      <div className="flex items-baseline gap-1">
                        <span className="text-4xl text-foreground tabular-nums sm:text-5xl">
                          {formatPriceWithCents(plan.prices.yearly)}
                        </span>
                        <span className="text-muted-foreground text-sm">
                          /yr
                        </span>
                      </div>
                      <p className="mt-1 text-muted-foreground/60 text-xs">
                        {priceDisplay}/mo
                      </p>
                    </>
                  ) : (
                    <>
                      <div className="flex items-baseline gap-1">
                        <span className="text-4xl text-foreground tabular-nums sm:text-5xl">
                          {priceDisplay}
                        </span>
                        {!(isEnterprise || isFree) && (
                          <span className="text-muted-foreground text-sm">
                            /mo
                          </span>
                        )}
                      </div>
                      {isFree && (
                        <p className="mt-1 text-muted-foreground/60 text-xs">
                          Free forever
                        </p>
                      )}
                    </>
                  )}
                </div>

                {plan.trial && (
                  <p className="mb-4 text-muted-foreground text-xs">
                    14-day free trial included
                  </p>
                )}

                <div className="mb-6 border-border/40 border-t" />

                {plan.computeCredit !== "100 runs/mo (micro, 10s)" && (
                  <p className="mb-4 font-medium text-foreground text-sm">
                    {plan.computeCredit} compute credit
                  </p>
                )}

                <ul className="mb-8 flex-1 space-y-3">
                  {plan.features.map((feature) => (
                    <li
                      className="flex items-start gap-3 text-sm leading-relaxed"
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

      <p className="mt-8 text-center text-muted-foreground/60 text-sm">
        All plans include core orchestration capabilities. Cancel anytime.
      </p>
    </div>
  );
}
