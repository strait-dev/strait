"use client";

import {
  ArrowRight02Icon,
  CheckmarkCircle02Icon,
} from "@hugeicons/core-free-icons";
import { HugeiconsIcon } from "@hugeicons/react";
import { Button } from "@strait/ui/components/button";
import Link from "next/link";
import { useState } from "react";

import Reveal from "@/components/landing/reveal.tsx";
import {
  StaggerGroup,
  StaggerItem,
} from "@/components/landing/stagger-group.tsx";
import Shell from "@/components/layout/shell.tsx";
import { dashboardHref } from "@/lib/urls.ts";

type PricingTier = {
  name: string;
  monthly: string;
  annual: string;
  annualNote: string;
  description: string;
  features: string[];
  cta: { label: string; href: string };
  highlighted: boolean;
};

const PRICING_TIERS: PricingTier[] = [
  {
    name: "Free",
    monthly: "$0",
    annual: "$0",
    annualNote: "",
    description: "All core features, no credit card required.",
    features: [
      "5,000 runs/day",
      "100 managed runs/mo",
      "2 projects, 3 members",
      "1 day retention",
      "All features included",
    ],
    cta: { label: "Get Started Free", href: "/login" },
    highlighted: false,
  },
  {
    name: "Starter",
    monthly: "$19.99",
    annual: "$16.67",
    annualNote: "Billed $199.99/yr",
    description: "14-day free trial. Ship to production with room to grow.",
    features: [
      "25,000 runs/day",
      "$19.99 compute credit/mo",
      "5 projects, 10 members",
      "7 day retention",
      "6 regions, configurable limits",
    ],
    cta: { label: "Start Free Trial", href: "/login" },
    highlighted: false,
  },
  {
    name: "Pro",
    monthly: "$49.99",
    annual: "$41.67",
    annualNote: "Billed $499.99/yr",
    description: "14-day free trial. For teams running production workloads.",
    features: [
      "100,000 runs/day",
      "$49.99 compute credit/mo",
      "15 projects, 25 members",
      "30 day retention",
      "All regions, full RBAC, audit logs",
    ],
    cta: { label: "Start Free Trial", href: "/login" },
    highlighted: true,
  },
  {
    name: "Enterprise",
    monthly: "Custom",
    annual: "Custom",
    annualNote: "",
    description: "Dedicated infrastructure, SLA, and SSO/SAML.",
    features: [
      "Unlimited runs",
      "Custom compute credits",
      "Unlimited projects & members",
      "90 day retention",
      "SSO/SAML, dedicated support",
    ],
    cta: { label: "Contact Sales", href: "/contact" },
    highlighted: false,
  },
];

const PricingTeaser = () => {
  const [annual, setAnnual] = useState(false);

  return (
    <section className="py-20 sm:py-28">
      <Shell variant="wide">
        <Reveal variant="blur">
          <div className="mb-14 max-w-3xl">
            <h2 className="text-balance text-2xl leading-[1.2] sm:text-3xl lg:text-4xl">
              Start free. Scale when you&apos;re ready.
            </h2>
            <p className="mt-4 text-pretty text-base text-muted-foreground leading-relaxed sm:text-lg">
              Every plan includes all core features. Pay only for scale and
              governance.
            </p>
          </div>
        </Reveal>

        <div className="mb-8 flex flex-col gap-6 sm:flex-row sm:items-center sm:justify-between">
          <StaggerGroup className="flex flex-wrap items-center gap-2.5">
            <StaggerItem>
              <span className="rounded-full border border-border/60 bg-card px-3 py-1 text-muted-foreground text-sm">
                No credit card for Free
              </span>
            </StaggerItem>
            <StaggerItem>
              <span className="rounded-full border border-border/60 bg-card px-3 py-1 text-muted-foreground text-sm">
                14-day trial on paid plans
              </span>
            </StaggerItem>
            <StaggerItem>
              <span className="rounded-full border border-border/60 bg-card px-3 py-1 text-muted-foreground text-sm">
                Cancel anytime
              </span>
            </StaggerItem>
          </StaggerGroup>

          {/* Annual/Monthly toggle */}
          <div className="flex items-center gap-3">
            <span
              className={`text-sm ${annual ? "text-muted-foreground" : "font-medium text-foreground"}`}
            >
              Monthly
            </span>
            <button
              aria-checked={annual}
              aria-label={`Switch to ${annual ? "monthly" : "annual"} billing`}
              className="relative inline-flex h-6 w-11 shrink-0 cursor-pointer rounded-full border border-border/60 bg-muted transition-colors duration-200 focus-visible:outline focus-visible:outline-2 focus-visible:outline-primary focus-visible:outline-offset-2"
              onClick={() => setAnnual((a) => !a)}
              role="switch"
              type="button"
            >
              <span
                className={`pointer-events-none inline-block size-5 rounded-full bg-foreground shadow transition-transform duration-200 ${
                  annual ? "translate-x-5" : "translate-x-0"
                }`}
              />
            </button>
            <span
              className={`text-sm ${annual ? "font-medium text-foreground" : "text-muted-foreground"}`}
            >
              Annual
            </span>
            {annual && (
              <span className="rounded-full bg-success/10 px-2 py-0.5 font-medium text-success text-xs">
                Save ~17%
              </span>
            )}
          </div>
        </div>

        <div className="grid grid-cols-1 gap-6 sm:grid-cols-2 lg:grid-cols-4 lg:gap-5">
          {PRICING_TIERS.map((plan, idx) => (
            <Reveal delay={idx * 0.08} key={plan.name} spring variant="scale">
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
                  <div className="mt-6 mb-6 sm:mt-8 sm:mb-8">
                    <span className="text-4xl text-foreground tabular-nums sm:text-5xl">
                      {annual ? plan.annual : plan.monthly}
                    </span>
                    {plan.monthly !== "$0" && plan.monthly !== "Custom" ? (
                      <span className="ml-1 text-muted-foreground text-sm">
                        /mo
                      </span>
                    ) : null}
                    {annual && plan.annualNote ? (
                      <p className="mt-1 text-muted-foreground/60 text-xs">
                        {plan.annualNote}
                      </p>
                    ) : null}
                  </div>

                  <ul className="mb-8 flex-1 space-y-3 sm:mb-10 sm:space-y-3.5">
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

        <Reveal delay={0.3}>
          <div className="mt-10 flex flex-col items-center gap-3 text-center">
            <p className="max-w-lg text-muted-foreground text-sm">
              All paid plans include compute credits equal to your subscription
              price. Overage at $0.20 per 1,000 runs on paid plans.
            </p>
            <Link
              className="font-medium text-muted-foreground text-sm transition-colors hover:text-foreground"
              href="/pricing"
            >
              See Full Pricing & Compare Plans →
            </Link>
          </div>
        </Reveal>
      </Shell>
    </section>
  );
};

export default PricingTeaser;
