"use client";

import { ArrowRight02Icon } from "@hugeicons/core-free-icons";
import { HugeiconsIcon } from "@hugeicons/react";
import { Button } from "@strait/ui/components/button";
import Link from "next/link";
import { useRef } from "react";

import SmoothCursor from "@/components/cultui/smooth-cursor.tsx";
import MeshGradientBg from "@/components/landing/mesh-gradient-bg.tsx";
import Reveal from "@/components/landing/reveal.tsx";
import Shell from "@/components/layout/shell.tsx";
import Particles from "@/components/magicui/particles.tsx";
import { dashboardHref } from "@/lib/urls.ts";

const CTA = () => {
  const headingId = "cta-title";
  const sectionRef = useRef<HTMLElement>(null);

  return (
    <section
      aria-labelledby={headingId}
      className="relative border-border/40 border-y bg-primary py-20 sm:py-28"
      ref={sectionRef}
    >
      <SmoothCursor containerRef={sectionRef} />
      <div className="orchestration-grid pointer-events-none absolute inset-0 opacity-[0.12]" />
      <MeshGradientBg />
      <Particles
        className="pointer-events-none absolute inset-0"
        color="var(--background)"
        quantity={80}
        size={0.4}
        staticity={40}
      />

      <Shell className="relative z-10" variant="wide">
        <div className="flex flex-col items-center text-center">
          <Reveal variant="blur">
            <h2
              className="max-w-3xl text-balance text-2xl text-primary-foreground leading-[1.1] sm:text-3xl lg:text-4xl"
              id={headingId}
            >
              Ship your first workflow in 10 minutes.
            </h2>
          </Reveal>

          <Reveal delay={0.1}>
            <p className="mt-6 max-w-2xl text-pretty text-base text-primary-foreground/70 leading-relaxed sm:text-lg">
              Connect your Postgres, define a workflow, and trigger your first
              run. No broker, no config files, no vendor lock-in.
            </p>
          </Reveal>

          <Reveal delay={0.2} spring>
            <div className="mt-10 flex flex-col items-center gap-4">
              <Button
                className="transition-all duration-300 hover:animate-gradient-shimmer"
                render={<Link href={dashboardHref("/login")} />}
                variant="outline"
              >
                Start building free
                <HugeiconsIcon className="size-4" icon={ArrowRight02Icon} />
              </Button>
              <p className="text-primary-foreground/50 text-sm">
                No credit card required. Runs on your Postgres.
              </p>
            </div>
          </Reveal>
        </div>
      </Shell>
    </section>
  );
};

export default CTA;
