import {
  ArrowRight02Icon,
  CheckmarkCircle02Icon,
} from "@hugeicons/core-free-icons";
import { HugeiconsIcon } from "@hugeicons/react";
import { PLANS as BILLING_PLANS, formatPrice } from "@strait/billing/products";
import { Button } from "@strait/ui/components/button";
import Link from "next/link";

import Reveal from "@/components/landing/reveal.tsx";
import {
  StaggerGroup,
  StaggerItem,
} from "@/components/landing/stagger-group.tsx";
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
      <Reveal variant="blur">
        <div className="mb-14 max-w-3xl">
          <h2 className="text-balance text-2xl leading-[1.2] sm:text-3xl lg:text-4xl">
            <span className="text-foreground">
              Free to start. Pay as you grow.
            </span>{" "}
            <span className="text-muted-foreground">
              No credit card required. Self-host or let us run it for you.
            </span>
          </h2>
        </div>
      </Reveal>

      <StaggerGroup className="mb-8 flex flex-wrap items-center justify-center gap-2.5 md:justify-start">
        <StaggerItem>
          <span className="rounded-full border border-border/60 bg-card px-3 py-1 text-muted-foreground text-sm">
            No credit card to start
          </span>
        </StaggerItem>
        <StaggerItem>
          <span className="rounded-full border border-border/60 bg-card px-3 py-1 text-muted-foreground text-sm">
            Cancel anytime
          </span>
        </StaggerItem>
        <StaggerItem>
          <span className="rounded-full border border-border/60 bg-card px-3 py-1 text-muted-foreground text-sm">
            Runs on your Postgres
          </span>
        </StaggerItem>
      </StaggerGroup>

      <div className="mx-auto grid max-w-4xl grid-cols-1 gap-6 md:grid-cols-2 lg:gap-8">
        {TEASER_PLANS.map((plan, idx) => (
          <Reveal delay={idx * 0.1} key={plan.name} spring variant="scale">
            <div
              className={`relative flex h-full flex-col overflow-hidden rounded-2xl border ${
                plan.highlighted
                  ? "border-primary/60 bg-card shadow-md"
                  : "border-border/60 bg-card"
              }`}
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
                  <span className="text-5xl text-foreground tabular-nums">
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
          </Reveal>
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
