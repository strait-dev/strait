"use client";

import {
  ArrowRight02Icon,
  CheckmarkCircle02Icon,
} from "@hugeicons/core-free-icons";
import { HugeiconsIcon } from "@hugeicons/react";
import { formatPrice, PLANS } from "@strait/billing/products";
import { Button } from "@strait/ui/components/button";
import { cn } from "@strait/ui/utils";
import Link from "next/link";
import { useMemo, useState } from "react";

import { dashboardHref } from "@/lib/urls.ts";

interface PricingPlan {
  cta: string;
  description: string;
  features: string[];
  id: string;
  monthlyPrice: number;
  name: string;
  popular?: boolean;
  yearlyPrice: number;
}

const staticPlans: PricingPlan[] = [
  {
    id: "personal",
    name: PLANS.personal.name,
    monthlyPrice: PLANS.personal.prices.monthly,
    yearlyPrice: PLANS.personal.prices.yearly,
    description: PLANS.personal.description,
    features: [...PLANS.personal.features],
    cta: "Start Personal",
  },
  {
    id: "pro",
    name: PLANS.pro.name,
    monthlyPrice: PLANS.pro.prices.monthly,
    yearlyPrice: PLANS.pro.prices.yearly,
    description: PLANS.pro.description,
    features: [...PLANS.pro.features],
    cta: "Start Pro",
    popular: true,
  },
];

export function StaticPricingTable() {
  const [interval, setInterval] = useState<"monthly" | "yearly">("yearly");

  const savingsPercent = useMemo(() => {
    const monthly = PLANS.personal.prices.monthly;
    const yearly = PLANS.personal.prices.yearly;
    return Math.round(((monthly - yearly) / monthly) * 100);
  }, []);

  return (
    <div className="mt-10 sm:mt-12">
      <div className="flex justify-center px-1">
        <div className="inline-flex items-center gap-1 rounded-full border border-border/60 bg-card p-1">
          <button
            className={cn(
              "min-h-11 rounded-full px-5 py-2.5 font-medium text-sm transition-all",
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
              "min-h-11 rounded-full px-5 py-2.5 font-medium text-sm transition-all",
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
            Save {savingsPercent}%
          </span>
        </div>
      </div>

      <div className="mt-10 grid grid-cols-1 gap-6 md:grid-cols-2 lg:gap-8 xl:gap-10">
        {staticPlans.map((plan) => {
          const price =
            interval === "monthly" ? plan.monthlyPrice : plan.yearlyPrice;

          return (
            <div
              className={cn(
                "relative flex h-full flex-col overflow-hidden rounded-2xl border transition-shadow duration-300",
                plan.popular
                  ? "border-primary/40"
                  : "border-border/60 bg-card hover:border-border hover:shadow-md"
              )}
              key={plan.id}
            >
              {plan.popular ? (
                <div className="relative bg-primary px-6 py-8 sm:px-8">
                  <div className="showcase-dots pointer-events-none absolute inset-0" />
                  <div
                    className="pointer-events-none absolute inset-0 opacity-30"
                    style={{
                      background:
                        "radial-gradient(circle at 30% 20%, oklch(1 0 0 / 0.2), transparent 50%), radial-gradient(circle at 70% 80%, oklch(1 0 0 / 0.1), transparent 50%)",
                    }}
                  />
                  <div className="relative z-10">
                    <span className="mb-4 inline-block rounded-md bg-primary-foreground/20 px-3 py-1.5 font-medium text-primary-foreground text-xs backdrop-blur-sm">
                      Most popular
                    </span>
                    <h3 className="text-2xl text-primary-foreground tracking-tight">
                      {plan.name}
                    </h3>
                    <p className="mt-2 max-w-sm text-pretty text-primary-foreground/70 text-sm leading-relaxed">
                      {plan.description}
                    </p>
                  </div>
                </div>
              ) : (
                <div className="px-6 pt-8 sm:px-8">
                  <h3 className="text-2xl text-foreground tracking-tight">
                    {plan.name}
                  </h3>
                  <p className="mt-2 max-w-sm text-pretty text-muted-foreground text-sm leading-relaxed">
                    {plan.description}
                  </p>
                </div>
              )}

              <div className="flex flex-1 flex-col px-6 pb-8 sm:px-8">
                <div className="mt-8 mb-8 flex items-baseline gap-1">
                  <span className="text-5xl text-foreground tabular-nums tracking-tight">
                    {formatPrice(price)}
                  </span>
                  <span className="text-muted-foreground text-sm">
                    /{interval === "monthly" ? "mo" : "mo billed yearly"}
                  </span>
                </div>

                <div className="mb-8 border-border/40 border-t" />

                <ul className="mb-10 flex-1 space-y-3.5">
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
                  className={cn("w-full", "transition-all duration-300")}
                  render={<Link href={dashboardHref("/login")} />}
                  variant={plan.popular ? "default" : "outline"}
                >
                  {plan.cta}
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
