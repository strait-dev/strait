"use client";

import { ArrowRight02Icon } from "@hugeicons/core-free-icons";
import { HugeiconsIcon } from "@hugeicons/react";
import { Button } from "@strait/ui/components/button";
import Link from "next/link";
import HeroDag from "@/components/landing/hero-dag.tsx";
import Reveal from "@/components/landing/reveal.tsx";
import {
  StaggerGroup,
  StaggerItem,
} from "@/components/landing/stagger-group.tsx";
import Shell from "@/components/layout/shell.tsx";
import HyperText from "@/components/magicui/hyper-text.tsx";
import { dashboardHref } from "@/lib/urls.ts";

const Hero = () => (
  <section className="relative isolate overflow-hidden pt-32 pb-12 sm:pt-40 sm:pb-16">
    <div className="parallax-slow absolute inset-0 -z-10 bg-[linear-gradient(to_bottom,_var(--primary)/0.06,_transparent_40%)]" />
    <div className="orchestration-grid absolute inset-0 -z-10 opacity-[0.14]" />
    <div className="absolute inset-0 -z-10 bg-[linear-gradient(to_bottom,_transparent,_var(--background)_70%)]" />
    <div className="paper-texture absolute inset-0 -z-10 opacity-[0.02]" />

    <Shell variant="wide">
      <div className="flex flex-col items-center gap-12 lg:flex-row lg:items-start lg:gap-16">
        {/* Text — 45% */}
        <div className="flex max-w-2xl flex-col lg:w-[45%]">
          <Reveal delay={0}>
            <span className="kicker">OPEN SOURCE JOB ORCHESTRATION</span>
          </Reveal>

          <h1 className="mt-6 text-balance text-4xl leading-[1.12] tracking-[-0.025em] sm:text-5xl lg:text-6xl">
            <HyperText
              as="span"
              className="text-foreground"
              duration={800}
              startOnView
            >
              Ship background workflows that don't wake you up at 3 AM.
            </HyperText>{" "}
            <span className="text-muted-foreground">
              Reliable queueing, workflow orchestration, and automatic failure
              recovery — all backed by your existing database.
            </span>
          </h1>

          <Reveal delay={0.2} spring>
            <p className="mt-5 max-w-xl text-pretty text-base text-muted-foreground/70 leading-relaxed sm:mt-6 sm:text-lg">
              Define jobs, wire dependencies, and let Strait handle retries,
              dead letters, approvals, and cost budgets — all backed by your
              existing Postgres.
            </p>
          </Reveal>

          <Reveal delay={0.3} spring>
            <div className="mt-10 flex flex-col items-start gap-4 sm:flex-row">
              <Button
                className="transition-shadow duration-300"
                render={<Link href={dashboardHref("/login")} />}
                variant="gradient"
              >
                Deploy your first workflow
                <HugeiconsIcon className="size-4" icon={ArrowRight02Icon} />
              </Button>
              <Button render={<Link href="/docs/quickstart" />} variant="ghost">
                Read the docs
                <HugeiconsIcon className="size-4" icon={ArrowRight02Icon} />
              </Button>
            </div>
          </Reveal>

          <StaggerGroup
            className="mt-6 flex flex-wrap items-start gap-2.5"
            delay={0.06}
          >
            <StaggerItem>
              <span className="rounded-full border border-border/60 bg-card px-3 py-1 text-muted-foreground text-sm">
                Built on Postgres
              </span>
            </StaggerItem>
            <StaggerItem>
              <span className="rounded-full border border-border/60 bg-card px-3 py-1 text-muted-foreground text-sm">
                Full lifecycle tracking
              </span>
            </StaggerItem>
            <StaggerItem>
              <span className="rounded-full border border-border/60 bg-card px-3 py-1 text-muted-foreground text-sm">
                Apache 2.0 licensed
              </span>
            </StaggerItem>
          </StaggerGroup>
        </div>

        {/* DAG Visual — 55% */}
        <Reveal
          className="relative w-full lg:w-[55%]"
          delay={0.2}
          spring
          variant="scale"
        >
          <div className="relative aspect-[4/3] overflow-hidden rounded-2xl border border-border/40 bg-card/50 backdrop-blur-sm">
            <div
              className="pointer-events-none absolute inset-0 opacity-20"
              style={{
                background:
                  "radial-gradient(circle at 50% 40%, var(--primary), transparent 70%)",
              }}
            />
            <div className="orchestration-grid pointer-events-none absolute inset-0 opacity-[0.06]" />
            <HeroDag />
          </div>
        </Reveal>
      </div>
    </Shell>
  </section>
);

export default Hero;
