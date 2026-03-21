import { ArrowRight02Icon } from "@hugeicons/core-free-icons";
import { HugeiconsIcon } from "@hugeicons/react";
import { Button } from "@strait/ui/components/button";
import Link from "next/link";
import Reveal from "@/components/landing/reveal.tsx";
import Shell from "@/components/layout/shell.tsx";
import { siteConfig } from "@/config/site.ts";
import { dashboardHref } from "@/lib/urls.ts";
import HeroProductPreview from "./hero-product-preview.tsx";

const Hero = () => {
  return (
    <section className="relative isolate overflow-hidden pt-32 pb-16 sm:pt-40 sm:pb-24">
      <Shell variant="wide">
        <div className="mx-auto flex max-w-4xl flex-col items-center text-center">
          <Reveal delay={0}>
            <span className="kicker">
              SHIP PRODUCTS, NOT JOB INFRASTRUCTURE
            </span>
          </Reveal>

          <Reveal delay={0.1} variant="blur">
            <h1 className="mt-6 text-balance text-4xl leading-[1.12] sm:text-5xl lg:text-6xl">
              Background jobs, workflows, and{" "}
              <span className="text-primary">AI agents</span> that never lose
              state.
            </h1>
            <p className="mx-auto mt-4 max-w-2xl text-pretty text-muted-foreground text-sm leading-relaxed sm:text-base">
              One platform to queue, orchestrate, and observe every async
              workload. Open-source, self-hostable, written in Go.
            </p>
          </Reveal>

          <Reveal delay={0.2} spring>
            <div className="mt-10 flex flex-col items-center gap-4 sm:flex-row">
              <Button
                render={<Link href={dashboardHref("/login")} />}
                size="default"
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
