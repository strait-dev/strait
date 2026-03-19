"use client";

import { ArrowRight02Icon } from "@hugeicons/core-free-icons";
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
import { siteConfig } from "@/config/site.ts";
import { dashboardHref } from "@/lib/urls.ts";

const CODE_TABS = [
  {
    label: "TypeScript",
    code: [
      { text: "import", kind: "keyword" },
      { text: " { Strait } ", kind: "default" },
      { text: "from", kind: "keyword" },
      { text: ' "@strait/sdk"', kind: "string" },
      { text: ";", kind: "default" },
      { text: "\n\n", kind: "default" },
      { text: "const", kind: "keyword" },
      { text: " strait = ", kind: "default" },
      { text: "new", kind: "keyword" },
      {
        text: " Strait({ apiKey: process.env.STRAIT_API_KEY });",
        kind: "default",
      },
      { text: "\n\n", kind: "default" },
      { text: "await", kind: "keyword" },
      { text: " strait.jobs.trigger(", kind: "default" },
      { text: '"process-order"', kind: "string" },
      { text: ", {\n", kind: "default" },
      { text: "  payload: { orderId: ", kind: "default" },
      { text: '"ord_a3f9"', kind: "string" },
      { text: ", amount: 49.99 },\n", kind: "default" },
      { text: "});", kind: "default" },
    ],
  },
  {
    label: "Python",
    code: [
      { text: "from", kind: "keyword" },
      { text: " strait ", kind: "default" },
      { text: "import", kind: "keyword" },
      { text: " Strait", kind: "default" },
      { text: "\n\n", kind: "default" },
      { text: "strait = Strait(api_key=os.environ[", kind: "default" },
      { text: '"STRAIT_API_KEY"', kind: "string" },
      { text: "])", kind: "default" },
      { text: "\n\n", kind: "default" },
      { text: "strait.jobs.trigger(", kind: "default" },
      { text: '"process-order"', kind: "string" },
      { text: ",\n", kind: "default" },
      { text: "    payload={", kind: "default" },
      { text: '"orderId"', kind: "string" },
      { text: ": ", kind: "default" },
      { text: '"ord_a3f9"', kind: "string" },
      { text: ", ", kind: "default" },
      { text: '"amount"', kind: "string" },
      { text: ": 49.99},\n)", kind: "default" },
    ],
  },
  {
    label: "Go",
    code: [
      { text: "client := strait.New(os.Getenv(", kind: "default" },
      { text: '"STRAIT_API_KEY"', kind: "string" },
      { text: "))", kind: "default" },
      { text: "\n\n", kind: "default" },
      { text: "client.Jobs.Trigger(ctx, ", kind: "default" },
      { text: '"process-order"', kind: "string" },
      { text: ", strait.Payload{\n", kind: "default" },
      { text: "    ", kind: "default" },
      { text: '"orderId"', kind: "string" },
      { text: ": ", kind: "default" },
      { text: '"ord_a3f9"', kind: "string" },
      { text: ", ", kind: "default" },
      { text: '"amount"', kind: "string" },
      { text: ": 49.99,\n})", kind: "default" },
    ],
  },
] as const;

type CodeToken = { text: string; kind: string };

const CodeSnippet = ({ tokens }: { tokens: readonly CodeToken[] }) => (
  <pre className="overflow-x-auto font-mono text-sm leading-relaxed">
    <code>
      {tokens.map((token, i) => {
        let className = "text-foreground/80";
        if (token.kind === "keyword") {
          className = "text-primary";
        } else if (token.kind === "string") {
          className = "text-success";
        } else if (token.kind === "comment") {
          className = "text-muted-foreground";
        }
        return (
          <span
            className={className}
            key={`${String(i)}-${token.text.slice(0, 8)}`}
          >
            {token.text}
          </span>
        );
      })}
    </code>
  </pre>
);

const Hero = () => {
  const [activeTab, setActiveTab] = useState(0);

  return (
    <section className="relative isolate overflow-hidden pt-32 pb-16 sm:pt-40 sm:pb-24">
      <div className="parallax-slow absolute inset-0 -z-10 bg-[linear-gradient(to_bottom,_var(--primary)/0.06,_transparent_40%)]" />
      <div className="orchestration-grid absolute inset-0 -z-10 opacity-[0.14]" />
      <div className="absolute inset-0 -z-10 bg-[linear-gradient(to_bottom,_transparent,_var(--background)_70%)]" />
      <div className="paper-texture absolute inset-0 -z-10 opacity-[0.02]" />

      <Shell variant="wide">
        <div className="mx-auto flex max-w-4xl flex-col items-center text-center">
          <Reveal delay={0}>
            <span className="kicker">OPEN SOURCE JOB ORCHESTRATION</span>
          </Reveal>

          <Reveal delay={0.1} variant="blur">
            <h1 className="mt-6 text-balance text-4xl leading-[1.12] tracking-[-0.025em] sm:text-5xl lg:text-6xl">
              <span className="text-foreground">
                The infrastructure for background jobs, workflows, and AI
                agents.
              </span>{" "}
              <span className="text-muted-foreground">
                Open-source job orchestration with managed container execution,
                workflow DAGs, and built-in AI cost tracking. 5 SDKs. Self-host
                or cloud.
              </span>
            </h1>
          </Reveal>

          <Reveal delay={0.2} spring>
            <div className="mt-10 flex flex-col items-center gap-4 sm:flex-row">
              <Button
                className="transition-shadow duration-300"
                render={<Link href={dashboardHref("/login")} />}
                variant="gradient"
              >
                Get Started Free
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
                variant="ghost"
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
                5 SDKs
              </span>
            </StaggerItem>
          </StaggerGroup>
        </div>

        {/* Code snippet */}
        <Reveal className="mt-16 sm:mt-20" delay={0.4} spring variant="scale">
          <div className="mx-auto max-w-2xl overflow-hidden rounded-xl border border-border/40 bg-card shadow-2xl shadow-black/10">
            {/* Tab bar */}
            <div className="flex items-center gap-1.5 border-border/40 border-b bg-muted/30 px-4 py-2.5">
              <div className="mr-3 flex gap-1.5">
                <div className="size-3 rounded-full bg-foreground/10" />
                <div className="size-3 rounded-full bg-foreground/10" />
                <div className="size-3 rounded-full bg-foreground/10" />
              </div>
              {CODE_TABS.map((tab, i) => (
                <button
                  className={`rounded-md px-3 py-1 font-medium text-xs transition-colors ${
                    activeTab === i
                      ? "bg-foreground/10 text-foreground"
                      : "text-muted-foreground hover:text-foreground"
                  }`}
                  key={tab.label}
                  onClick={() => setActiveTab(i)}
                  type="button"
                >
                  {tab.label}
                </button>
              ))}
            </div>
            {/* Code content */}
            <div className="p-5 sm:p-6">
              <CodeSnippet
                tokens={CODE_TABS[activeTab]?.code ?? CODE_TABS[0].code}
              />
            </div>
          </div>
        </Reveal>
      </Shell>
    </section>
  );
};

export default Hero;
