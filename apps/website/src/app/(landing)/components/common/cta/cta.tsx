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
              Get started in 5 minutes
            </h2>
          </Reveal>

          <Reveal delay={0.1}>
            <div className="mt-8 overflow-hidden rounded-lg border border-primary-foreground/20 bg-primary-foreground/10 px-6 py-3">
              <pre className="font-mono text-primary-foreground/90 text-sm">
                <code>
                  npm install @strait/ts{" "}
                  <span className="text-primary-foreground/40">
                    # or pip, go get, gem, cargo
                  </span>
                </code>
              </pre>
            </div>
          </Reveal>

          <Reveal delay={0.2} spring>
            <div className="mt-10 flex flex-col items-center gap-4 sm:flex-row">
              <Button
                className="transition-all duration-300 hover:animate-gradient-shimmer"
                render={<Link href={dashboardHref("/login")} />}
                variant="outline"
              >
                Get Started Free
                <HugeiconsIcon className="size-4" icon={ArrowRight02Icon} />
              </Button>
              <Button
                render={<Link href="/docs/quickstart" />}
                variant="outline"
              >
                Read the Docs
                <HugeiconsIcon className="size-4" icon={ArrowRight02Icon} />
              </Button>
              <Button
                render={
                  // biome-ignore lint/a11y/useAnchorContent: content provided by Button children
                  <a
                    aria-label="Join Discord"
                    href="https://discord.gg/strait"
                    rel="noopener noreferrer"
                    target="_blank"
                  />
                }
                variant="outline"
              >
                Join Discord
                <HugeiconsIcon className="size-4" icon={ArrowRight02Icon} />
              </Button>
            </div>
          </Reveal>

          <Reveal delay={0.3}>
            <p className="mt-6 text-primary-foreground/50 text-sm">
              No credit card required. Self-host or cloud.
            </p>
          </Reveal>
        </div>
      </Shell>
    </section>
  );
};

export default CTA;
