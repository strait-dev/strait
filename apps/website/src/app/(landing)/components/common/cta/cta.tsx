import { ArrowRight02Icon } from "@hugeicons/core-free-icons";
import { HugeiconsIcon } from "@hugeicons/react";
import { Button } from "@strait/ui/components/button";
import Link from "next/link";

import Reveal from "@/components/landing/reveal.tsx";
import Shell from "@/components/layout/shell.tsx";
import { dashboardHref } from "@/lib/urls.ts";

type CTAProps = {
  heading?: string;
  description?: string;
  showInstallSnippet?: boolean;
};

const CTA = ({
  heading = "Run your first job in 5 minutes",
  description = "Free tier included. No credit card required.",
  showInstallSnippet = true,
}: CTAProps) => {
  const headingId = "cta-title";

  return (
    <section
      aria-labelledby={headingId}
      className="relative overflow-hidden border-border/40 border-y py-20 sm:py-28"
    >
      <div className="absolute inset-0 -z-10 bg-primary/[0.03]" />

      <Shell className="relative z-10" variant="wide">
        <div className="max-w-3xl">
          <Reveal variant="blur">
            <h2
              className="text-balance text-2xl leading-[1.1] sm:text-3xl lg:text-4xl"
              id={headingId}
            >
              {heading}
            </h2>
          </Reveal>

          {showInstallSnippet && (
            <Reveal delay={0.1}>
              <div className="mt-8 inline-block overflow-hidden rounded-lg border border-border/40 bg-muted/30 px-6 py-3">
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
          )}

          <Reveal delay={showInstallSnippet ? 0.2 : 0.1} spring>
            <div className="mt-10 flex flex-col gap-4 sm:flex-row">
              <Button
                render={<Link href={dashboardHref("/login")} />}
                size="default"
                variant="gradient"
              >
                Start Building Free
                <HugeiconsIcon className="size-4" icon={ArrowRight02Icon} />
              </Button>
              <Button
                render={<Link href="/docs/quickstart" />}
                size="default"
                variant="ghost"
              >
                Read the Docs
                <HugeiconsIcon className="size-4" icon={ArrowRight02Icon} />
              </Button>
            </div>
          </Reveal>

          <Reveal delay={showInstallSnippet ? 0.3 : 0.2}>
            <p className="mt-6 text-muted-foreground/60 text-sm">
              {description}
            </p>
          </Reveal>
        </div>
      </Shell>
    </section>
  );
};

export default CTA;
