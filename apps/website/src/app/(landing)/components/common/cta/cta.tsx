import { ArrowRight02Icon } from "@hugeicons/core-free-icons";
import { HugeiconsIcon } from "@hugeicons/react";
import { Button } from "@strait/ui/components/button";
import Link from "next/link";

import Reveal from "@/components/landing/reveal.tsx";
import Shell from "@/components/layout/shell.tsx";
import { dashboardHref } from "@/lib/urls.ts";

const CTA = () => {
  const headingId = "cta-title";

  return (
    <section
      aria-labelledby={headingId}
      className="relative overflow-hidden border-border/40 border-y py-20 sm:py-28"
    >
      {/* Background effects */}
      <div className="absolute inset-0 -z-10 bg-[linear-gradient(to_bottom,_transparent,_var(--primary)/0.06,_transparent)]" />
      <div className="absolute inset-0 -z-10 bg-[radial-gradient(ellipse_80%_60%_at_50%_50%,_var(--primary)/0.08,_transparent)]" />
      <div className="orchestration-grid pointer-events-none absolute inset-0 opacity-[0.08]" />

      <Shell className="relative z-10" variant="wide">
        <div className="flex flex-col items-center text-center">
          <Reveal variant="blur">
            <h2
              className="max-w-3xl text-balance text-2xl leading-[1.1] sm:text-3xl lg:text-4xl"
              id={headingId}
            >
              Run your first job in 5 minutes
            </h2>
          </Reveal>

          <Reveal delay={0.1}>
            <div className="mt-8 overflow-hidden rounded-lg border border-border/40 bg-muted/30 px-6 py-3">
              <pre className="font-mono text-foreground/80 text-sm">
                <code>
                  npm install @strait/ts{" "}
                  <span className="text-muted-foreground/50">
                    # or pip, go get, gem, cargo
                  </span>
                </code>
              </pre>
            </div>
          </Reveal>

          <Reveal delay={0.2} spring>
            <div className="mt-10 flex flex-col items-center gap-4 sm:flex-row">
              <Button
                className="transition-shadow duration-300"
                render={<Link href={dashboardHref("/login")} />}
                variant="gradient"
              >
                Start Building Free
                <HugeiconsIcon className="size-4" icon={ArrowRight02Icon} />
              </Button>
              <Button render={<Link href="/docs/quickstart" />} variant="ghost">
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
                variant="ghost"
              >
                Join Discord
                <HugeiconsIcon className="size-4" icon={ArrowRight02Icon} />
              </Button>
            </div>
          </Reveal>

          <Reveal delay={0.3}>
            <p className="mt-6 text-muted-foreground text-sm">
              Free tier included. No credit card required.
            </p>
          </Reveal>
        </div>
      </Shell>
    </section>
  );
};

export default CTA;
