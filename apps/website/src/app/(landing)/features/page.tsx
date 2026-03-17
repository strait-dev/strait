import { ArrowRight02Icon } from "@hugeicons/core-free-icons";
import { HugeiconsIcon } from "@hugeicons/react";
import { Button } from "@strait/ui/components/button";
import Link from "next/link";

import Reveal from "@/components/landing/reveal.tsx";
import {
  StaggerGroup,
  StaggerItem,
} from "@/components/landing/stagger-group.tsx";
import Shell from "@/components/layout/shell.tsx";
import FlickeringGrid from "@/components/magicui/flickering-grid.tsx";
import { generateMetadata as generatePageMetadata } from "@/lib/metadata.ts";
import { dashboardHref } from "@/lib/urls.ts";
import { FEATURE_PAGES } from "./data.ts";

export const metadata = generatePageMetadata({
  title: "Features — Strait",
  description:
    "Explore Strait features: PostgreSQL queue, workflow DAGs, retries, approval gates, cost budgets, real-time CDC, and SDK endpoints.",
  path: "/features",
  keywords: [
    "Strait features",
    "job orchestration features",
    "PostgreSQL queue",
    "workflow DAGs",
  ],
});

export default function FeaturesPage() {
  return (
    <>
      <section className="relative isolate overflow-hidden pt-32 pb-16 sm:pt-40 sm:pb-20">
        <div className="orchestration-grid absolute inset-0 -z-10 opacity-[0.08]" />
        <div className="absolute inset-0 -z-10 bg-[linear-gradient(to_bottom,_transparent,_var(--background)_70%)]" />

        <Shell variant="wide">
          <div className="max-w-3xl">
            <span className="kicker">Features</span>
            <Reveal variant="blur">
              <h1 className="mt-4 text-4xl leading-[1.12] tracking-[-0.025em] sm:text-5xl lg:text-6xl">
                <span className="text-foreground">
                  Everything you need to run production workflows.
                </span>
              </h1>
            </Reveal>
            <p className="mt-6 max-w-2xl text-lg text-muted-foreground/70 leading-relaxed">
              A complete runtime with queueing, orchestration, and operations
              built in. Not a framework — a production-grade platform.
            </p>
          </div>
        </Shell>
      </section>

      <section className="py-16 sm:py-20">
        <Shell variant="wide">
          <StaggerGroup className="grid grid-cols-1 gap-6 sm:grid-cols-2 lg:grid-cols-3">
            {FEATURE_PAGES.map((feature) => (
              <StaggerItem key={feature.slug}>
                <Link
                  className="group flex h-full flex-col rounded-2xl border border-border/60 bg-card p-6 transition-shadow hover:shadow-md sm:p-8"
                  href={`/features/${feature.slug}`}
                >
                  <h2 className="font-semibold text-foreground text-xl group-hover:text-primary">
                    {feature.name}
                  </h2>
                  <p className="mt-1 text-muted-foreground text-sm">
                    {feature.subheadline}
                  </p>
                  <p className="mt-4 flex-1 text-muted-foreground/70 text-sm leading-relaxed">
                    {feature.description}
                  </p>
                  <div className="mt-4 flex items-center gap-1 text-primary text-sm">
                    Learn more
                    <HugeiconsIcon
                      className="size-3.5"
                      icon={ArrowRight02Icon}
                    />
                  </div>
                </Link>
              </StaggerItem>
            ))}
          </StaggerGroup>
        </Shell>
      </section>

      <section className="relative border-border/40 border-t bg-primary py-16 sm:py-20">
        <FlickeringGrid
          color="rgba(255,255,255,0.6)"
          flickerChance={0.2}
          maxOpacity={0.15}
        />
        <Shell className="relative z-10 text-center" variant="wide">
          <h2 className="text-2xl text-primary-foreground tracking-tight sm:text-3xl">
            Ready to deploy your first workflow?
          </h2>
          <p className="mt-4 text-primary-foreground/70">
            Zero-broker, production-grade from the first run.
          </p>
          <div className="mt-8">
            <Button
              render={<Link href={dashboardHref("/login")} />}
              variant="outline"
            >
              Get started
              <HugeiconsIcon className="size-4" icon={ArrowRight02Icon} />
            </Button>
          </div>
        </Shell>
      </section>
    </>
  );
}
