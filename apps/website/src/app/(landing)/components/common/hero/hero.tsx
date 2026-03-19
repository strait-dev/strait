"use client";

import { ArrowRight02Icon } from "@hugeicons/core-free-icons";
import { HugeiconsIcon } from "@hugeicons/react";
import { Button } from "@strait/ui/components/button";
import { AnimatePresence, motion } from "motion/react";
import Link from "next/link";
import { useCallback, useState } from "react";
import Reveal from "@/components/landing/reveal.tsx";
import {
  StaggerGroup,
  StaggerItem,
} from "@/components/landing/stagger-group.tsx";
import Shell from "@/components/layout/shell.tsx";
import { siteConfig } from "@/config/site.ts";
import { SPRING_SNAPPY } from "@/lib/motion.ts";
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

const CopyButton = ({ text }: { text: string }) => {
  const [copied, setCopied] = useState(false);

  const handleCopy = useCallback(() => {
    navigator.clipboard.writeText(text);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  }, [text]);

  return (
    <button
      aria-label="Copy code to clipboard"
      className="rounded-md p-1.5 text-muted-foreground/50 transition-colors hover:bg-foreground/5 hover:text-muted-foreground"
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
          <path
            d="M20 6L9 17l-5-5"
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
          <rect height="13" rx="2" width="13" x="9" y="9" />
          <path
            d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"
            strokeLinecap="round"
            strokeLinejoin="round"
          />
        </svg>
      )}
    </button>
  );
};

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

function getPlainText(tokens: readonly CodeToken[]): string {
  return tokens.map((t) => t.text).join("");
}

const Hero = () => {
  const [activeTab, setActiveTab] = useState(0);

  const handleTabKeyDown = useCallback(
    (e: React.KeyboardEvent) => {
      let next = activeTab;
      if (e.key === "ArrowRight") {
        next = (activeTab + 1) % CODE_TABS.length;
      } else if (e.key === "ArrowLeft") {
        next = (activeTab - 1 + CODE_TABS.length) % CODE_TABS.length;
      } else if (e.key === "Home") {
        next = 0;
      } else if (e.key === "End") {
        next = CODE_TABS.length - 1;
      } else {
        return;
      }
      e.preventDefault();
      setActiveTab(next);
      const btn =
        e.currentTarget.querySelectorAll<HTMLButtonElement>('[role="tab"]')[
          next
        ];
      btn?.focus();
    },
    [activeTab]
  );

  const currentTokens = CODE_TABS[activeTab]?.code ?? CODE_TABS[0].code;

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
                Written in Go
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
              <div
                aria-label="Code language"
                className="flex items-center gap-1.5"
                onKeyDown={handleTabKeyDown}
                role="tablist"
              >
                {CODE_TABS.map((tab, i) => (
                  <button
                    aria-controls={`hero-tabpanel-${String(i)}`}
                    aria-selected={activeTab === i}
                    className={`rounded-md px-3 py-1 font-medium text-xs transition-colors ${
                      activeTab === i
                        ? "bg-foreground/10 text-foreground"
                        : "text-muted-foreground hover:text-foreground"
                    }`}
                    id={`hero-tab-${String(i)}`}
                    key={tab.label}
                    onClick={() => setActiveTab(i)}
                    role="tab"
                    tabIndex={activeTab === i ? 0 : -1}
                    type="button"
                  >
                    {tab.label}
                  </button>
                ))}
              </div>
              <div className="ml-auto">
                <CopyButton text={getPlainText(currentTokens)} />
              </div>
            </div>
            {/* Code content */}
            <div
              aria-labelledby={`hero-tab-${String(activeTab)}`}
              className="p-5 sm:p-6"
              id={`hero-tabpanel-${String(activeTab)}`}
              role="tabpanel"
            >
              <AnimatePresence mode="wait">
                <motion.div
                  animate={{ opacity: 1, y: 0 }}
                  exit={{ opacity: 0, y: -8 }}
                  initial={{ opacity: 0, y: 8 }}
                  key={activeTab}
                  transition={SPRING_SNAPPY}
                >
                  <CodeSnippet tokens={currentTokens} />
                </motion.div>
              </AnimatePresence>
            </div>
          </div>
        </Reveal>
      </Shell>
    </section>
  );
};

export default Hero;
