import { ArrowRight02Icon } from "@hugeicons/core-free-icons";
import { HugeiconsIcon } from "@hugeicons/react";
import { Button } from "@strait/ui/components/button";
import { useState } from "react";
import Reveal from "@/components/landing/reveal.tsx";
import Shell from "@/components/layout/shell.tsx";
import { dashboardHref } from "@/lib/urls.ts";

type CTAProps = {
  heading?: string;
  description?: string;
  showInstallSnippet?: boolean;
};

const CopyButton = ({ text }: { text: string }) => {
  const [copied, setCopied] = useState(false);

  const handleCopy = async () => {
    await navigator.clipboard.writeText(text);
    setCopied(true);
    setTimeout(() => setCopied(false), 1500);
  };

  return (
    <button
      aria-label="Copy install command"
      className="shrink-0 rounded-md p-1 text-muted-foreground/60 transition-colors hover:text-foreground"
      onClick={handleCopy}
      type="button"
    >
      {copied ? (
        <svg
          className="size-4"
          fill="none"
          stroke="currentColor"
          strokeWidth={2}
          viewBox="0 0 24 24"
        >
          <polyline
            points="20 6 9 17 4 12"
            strokeLinecap="round"
            strokeLinejoin="round"
          />
        </svg>
      ) : (
        <svg
          className="size-4"
          fill="none"
          stroke="currentColor"
          strokeWidth={2}
          viewBox="0 0 24 24"
        >
          <rect height={13} rx={2} width={13} x={9} y={9} />
          <path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1" />
        </svg>
      )}
    </button>
  );
};

const CTA = ({
  heading = "Start running jobs in minutes",
  description = "Free forever on the starter plan. No credit card required.",
  showInstallSnippet = true,
}: CTAProps) => {
  const headingId = "cta-title";

  return (
    <section
      aria-labelledby={headingId}
      className="relative overflow-hidden border-border/40 border-y py-24 sm:py-32"
    >
      <div className="pointer-events-none absolute inset-0 -z-10 bg-[radial-gradient(ellipse_at_center,oklch(0.55_0.15_var(--primary-hue)/0.08),transparent_70%)]" />

      <Shell className="relative z-10" variant="wide">
        <div className="mx-auto max-w-3xl text-center">
          <Reveal variant="blur">
            <h2
              className="text-balance font-semibold text-3xl leading-[1.1] sm:text-4xl lg:text-5xl"
              id={headingId}
            >
              {heading}
            </h2>
          </Reveal>

          <Reveal delay={0.05}>
            <p className="mt-4 text-base text-muted-foreground">
              {description}
            </p>
          </Reveal>

          {showInstallSnippet && (
            <Reveal delay={0.1}>
              <div className="mt-8 inline-block overflow-hidden rounded-lg border border-border/40 bg-muted/30 px-6 py-3">
                <div className="flex items-center gap-3">
                  <pre className="font-mono text-foreground/80 text-sm">
                    <code>
                      npm install @strait/ts{" "}
                      <span className="text-muted-foreground/50">
                        # or pip, go get, gem, cargo
                      </span>
                    </code>
                  </pre>
                  <CopyButton text="npm install @strait/ts" />
                </div>
              </div>
            </Reveal>
          )}

          <Reveal delay={showInstallSnippet ? 0.2 : 0.1} spring>
            <div className="mt-10 flex flex-col items-center justify-center gap-4 sm:flex-row">
              <Button
                render={<a href={dashboardHref("/login")} />}
                size="default"
                variant="default"
              >
                Start building free
                <HugeiconsIcon className="size-4" icon={ArrowRight02Icon} />
              </Button>
              <Button
                render={<a href="/docs/quickstart" />}
                size="default"
                variant="ghost"
              >
                Read the docs
                <HugeiconsIcon className="size-4" icon={ArrowRight02Icon} />
              </Button>
            </div>
          </Reveal>
        </div>
      </Shell>
    </section>
  );
};

export default CTA;
