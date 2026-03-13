import {
  ArrowRight02Icon,
  CheckmarkCircle02Icon,
} from "@hugeicons/core-free-icons";
import { HugeiconsIcon } from "@hugeicons/react";
import { PLANS as BILLING_PLANS, formatPrice } from "@strait/billing/products";
import { Button } from "@strait/ui/components/button";
import Link from "next/link";

import Shell from "@/components/layout/shell.tsx";
import { dashboardHref } from "@/lib/urls.ts";

const TEASER_PLANS = [
  {
    key: "personal" as const,
    ...BILLING_PLANS.personal,
    cta: { label: `Start ${BILLING_PLANS.personal.name}`, href: "/login" },
    highlighted: false,
  },
  {
    key: "pro" as const,
    ...BILLING_PLANS.pro,
    cta: { label: `Go ${BILLING_PLANS.pro.name}`, href: "/login" },
    highlighted: true,
  },
];

const PricingTeaser = () => (
  <section className="py-20 sm:py-28">
    <Shell variant="wide">
      <div className="mb-14 max-w-3xl">
        <h2 className="text-balance text-2xl leading-[1.2] tracking-tight sm:text-3xl lg:text-4xl">
          <span className="font-bold text-foreground">
            Pricing that scales with your team.
          </span>{" "}
          <span className="text-muted-foreground">
            Start simple today, then unlock more power as your workflows grow.
          </span>
        </h2>
      </div>

      <div className="mb-8 flex flex-wrap items-center justify-center gap-2.5 md:justify-start">
        <span className="rounded-full border border-border/60 bg-card px-3 py-1 text-muted-foreground text-sm">
          Cancel anytime
        </span>
        <span className="rounded-full border border-border/60 bg-card px-3 py-1 text-muted-foreground text-sm">
          Keep your existing PostgreSQL setup
        </span>
        <span className="rounded-full border border-border/60 bg-card px-3 py-1 text-muted-foreground text-sm">
          Upgrade as your workload grows
        </span>
      </div>

      <div className="mx-auto grid max-w-4xl grid-cols-1 gap-6 md:grid-cols-2 lg:gap-8">
        {TEASER_PLANS.map((plan) => (
          <div
            className={`relative flex flex-col overflow-hidden rounded-2xl border ${
              plan.highlighted
                ? "border-foreground/10"
                : "border-border/60 bg-card"
            }`}
            key={plan.name}
          >
            {plan.highlighted ? (
              <div className="relative bg-primary px-6 py-8 sm:px-8">
                <div className="showcase-dots pointer-events-none absolute inset-0" />
                <div
                  className="pointer-events-none absolute inset-0 opacity-30"
                  style={{
                    background:
                      "radial-gradient(circle at 50% 40%, oklch(1 0 0 / 0.15), transparent 60%)",
                  }}
                />
                <div className="relative z-10">
                  <span className="mb-3 inline-block rounded-md bg-primary-foreground/20 px-2.5 py-1 font-medium text-primary-foreground text-xs">
                    Most popular
                  </span>
                  <h3 className="font-semibold text-lg text-primary-foreground">
                    {plan.name}
                  </h3>
                  <p className="mt-1 text-pretty text-primary-foreground/70 text-sm">
                    {plan.description}
                  </p>
                </div>
              </div>
            ) : (
              <div className="px-6 pt-6 sm:px-8 sm:pt-8">
                <h3 className="font-semibold text-foreground text-lg">
                  {plan.name}
                </h3>
                <p className="mt-1 text-pretty text-muted-foreground text-sm">
                  {plan.description}
                </p>
              </div>
            )}

            <div className="flex flex-1 flex-col px-6 pb-8 sm:px-8">
              <div className="mt-8 mb-8">
                <span className="font-bold text-5xl text-foreground tabular-nums tracking-tight">
                  {formatPrice(plan.prices.yearly)}
                </span>
                <span className="ml-1 text-muted-foreground text-sm">
                  /mo billed yearly
                </span>
              </div>

              <ul className="mb-10 flex-1 space-y-3.5">
                {plan.features.map((feature) => (
                  <li
                    className="flex items-start gap-2.5 text-sm"
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
                render={<Link href={dashboardHref(plan.cta.href)} />}
                variant={plan.highlighted ? "default" : "outline"}
              >
                {plan.cta.label}
                <HugeiconsIcon className="size-4" icon={ArrowRight02Icon} />
              </Button>
            </div>
          </div>
        ))}
      </div>

      <div className="mt-8 text-center">
        <Link
          className="font-medium text-muted-foreground text-sm transition-colors hover:text-foreground"
          href="/pricing"
        >
          Compare all plans in detail →
        </Link>
      </div>
    </Shell>
  </section>
);

export default PricingTeaser;
