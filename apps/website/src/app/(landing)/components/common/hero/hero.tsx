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
import { siteConfig } from "@/config/site.ts";
import { dashboardHref } from "@/lib/urls.ts";
import HeroProductPreview from "./hero-product-preview.tsx";

const Hero = () => {
  return (
    <section className="relative isolate overflow-hidden pt-32 pb-16 sm:pt-40 sm:pb-24">
      <div className="parallax-slow absolute inset-0 -z-10 bg-[linear-gradient(to_bottom,_var(--primary)/0.06,_transparent_40%)]" />
      <div className="orchestration-grid absolute inset-0 -z-10 opacity-[0.14]" />
      <div className="absolute inset-0 -z-10 bg-[linear-gradient(to_bottom,_transparent,_var(--background)_70%)]" />
      <div className="paper-texture absolute inset-0 -z-10 opacity-[0.02]" />

      <Shell variant="wide">
        <div className="mx-auto flex max-w-4xl flex-col items-center text-center">
          <Reveal delay={0}>
            <span className="kicker">
              SHIP PRODUCTS, NOT JOB INFRASTRUCTURE
            </span>
          </Reveal>

          <Reveal delay={0.1} variant="blur">
            <h1 className="mt-6 text-balance text-4xl leading-[1.12] sm:text-5xl lg:text-6xl">
              Background jobs, workflows, and AI agents that never lose state.
            </h1>
            <p className="mt-3 text-pretty text-muted-foreground text-sm leading-relaxed sm:text-base">
              One platform to queue, orchestrate, and observe every async
              workload. Open-source and self-hostable.
            </p>
          </Reveal>

          <Reveal delay={0.2} spring>
            <div className="mt-10 flex flex-col items-center gap-4 sm:flex-row">
              <Button
                className="transition-shadow duration-300"
                render={<Link href={dashboardHref("/login")} />}
                variant="gradient"
              >
                Run Your First Job
                <HugeiconsIcon className="size-4" icon={ArrowRight02Icon} />
              </Button>
              <Button
                render={
                  // biome-ignore lint/a11y/useAnchorContent: content provided by Button children
                  <a
                    aria-label="View on GitHub"
                    href={siteConfig.links.github}
                    rel="noopener noreferrer"
                    target="_blank"
                  />
                }
                size="default"
                variant="outline"
              >
                View on GitHub
                <HugeiconsIcon className="size-4" icon={ArrowRight02Icon} />
              </Button>
            </div>
          </Reveal>

          <StaggerGroup
            className="mt-6 flex flex-wrap items-center justify-center gap-2.5"
            delay={0.06}
          >
            <StaggerItem>
              <span className="rounded-full border border-border/60 bg-card px-3 py-1 text-muted-foreground text-sm">
                Open Source
              </span>
            </StaggerItem>
            <StaggerItem>
              <span className="rounded-full border border-border/60 bg-card px-3 py-1 text-muted-foreground text-sm">
                Apache 2.0
              </span>
            </StaggerItem>
            <StaggerItem>
              <span className="rounded-full border border-border/60 bg-card px-3 py-1 text-muted-foreground text-sm">
                Written in Go
              </span>
            </StaggerItem>
          </StaggerGroup>
        </div>

        {/* Interactive product preview */}
        <Reveal className="mt-16 sm:mt-20" delay={0.4} spring variant="scale">
          <HeroProductPreview />
        </Reveal>
      </Shell>
    </section>
  );
};

export default Hero;
