"use client";

import { AnimatePresence, motion } from "motion/react";
import { useCallback, useEffect, useRef, useState } from "react";
import { Prism as SyntaxHighlighter } from "react-syntax-highlighter";
import { oneDark } from "react-syntax-highlighter/dist/esm/styles/prism";
import Reveal from "@/components/landing/reveal.tsx";
import Shell from "@/components/layout/shell.tsx";
import MockBrowserWindow from "@/components/magicui/mock-browser-window.tsx";
import { SPRING_SNAPPY } from "@/lib/motion.ts";

const TABS = [
  {
    label: "Define a job",
    filename: "jobs/process-order.ts",
    language: "typescript",
    code: `import { defineJob } from "@strait/sdk";

export default defineJob("process-order", {
  retries: 3,
  backoff: "exponential",
  timeout: "30s",

  async handler(run) {
    const order = await db.orders.find(run.payload.orderId);
    await chargePayment(order);
    await sendConfirmation(order);
  },
});`,
  },
  {
    label: "Create a workflow",
    filename: "workflows/checkout.ts",
    language: "typescript",
    code: `import { defineWorkflow } from "@strait/sdk";

export default defineWorkflow("checkout-flow", {
  steps: {
    validate:  { job: "validate-order" },
    charge:   { job: "charge-payment", after: ["validate"] },
    approve:  { gate: "manual", after: ["charge"] },
    fulfill:  { job: "fulfill-order", after: ["approve"] },
    notify:   { job: "send-receipt", after: ["fulfill"] },
  },
});`,
  },
  {
    label: "AI agent guardrails",
    filename: "jobs/ai-research.ts",
    language: "typescript",
    code: `import { defineJob } from "@strait/sdk";

export default defineJob("ai-research-agent", {
  budget: { maxCost: "$5.00", model: "gpt-4o" },
  approvalRequired: true,
  retries: 1,

  async handler(run) {
    const result = await run.ai.complete({
      prompt: run.payload.query,
      tools: [searchWeb, readDocs],
    });
    return result;
    // cost is tracked automatically
  },
});`,
  },
  {
    label: "Stream with React",
    filename: "components/order-status.tsx",
    language: "tsx",
    code: `import { useRun } from "@strait/react";

export function OrderStatus({ runId }: { runId: string }) {
  const { status, steps, cost } = useRun(runId);

  return (
    <div>
      <p>Status: {status}</p>
      <p>Cost: \${cost.total}</p>
      {steps.map(step =>
        <Step key={step.id} {...step} />
      )}
    </div>
  );
}`,
  },
] as const;

const customTheme = {
  ...oneDark,
  'pre[class*="language-"]': {
    ...oneDark['pre[class*="language-"]'],
    background: "transparent",
    margin: 0,
    padding: 0,
    fontSize: "0.875rem",
    lineHeight: "1.625",
  },
  'code[class*="language-"]': {
    ...oneDark['code[class*="language-"]'],
    background: "transparent",
    fontSize: "0.875rem",
    lineHeight: "1.625",
  },
};

const CopyButton = ({ text }: { text: string }) => {
  const [copied, setCopied] = useState(false);
  const timerRef = useRef<ReturnType<typeof setTimeout>>(undefined);

  useEffect(() => {
    return () => clearTimeout(timerRef.current);
  }, []);

  const handleCopy = useCallback(() => {
    navigator.clipboard.writeText(text);
    setCopied(true);
    clearTimeout(timerRef.current);
    timerRef.current = setTimeout(() => setCopied(false), 2000);
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

const CodeExampleSection = () => {
  const [activeTab, setActiveTab] = useState(0);
  const currentTab = TABS[activeTab] ?? TABS[0];

  const handleTabKeyDown = useCallback(
    (e: React.KeyboardEvent) => {
      let next = activeTab;
      if (e.key === "ArrowRight") {
        next = (activeTab + 1) % TABS.length;
      } else if (e.key === "ArrowLeft") {
        next = (activeTab - 1 + TABS.length) % TABS.length;
      } else if (e.key === "Home") {
        next = 0;
      } else if (e.key === "End") {
        next = TABS.length - 1;
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

  return (
    <section className="bg-muted/30 py-20 sm:py-28">
      <Shell variant="wide">
        <Reveal variant="blur">
          <div className="mb-14 max-w-3xl">
            <h2 className="text-balance text-2xl leading-[1.2] sm:text-3xl lg:text-4xl">
              From idea to production in minutes.
            </h2>
            <p className="mt-3 text-pretty text-muted-foreground text-sm leading-relaxed sm:text-base">
              Define a job, wire it into a workflow, set cost guardrails for AI
              agents, and stream status to your frontend.
            </p>
          </div>
        </Reveal>

        {/* Tab bar */}
        <div
          aria-label="Code examples"
          className="-mx-1 mb-6 flex items-center gap-1.5 overflow-x-auto px-1 pb-2 sm:flex-wrap sm:gap-2 sm:overflow-visible sm:pb-0"
          onKeyDown={handleTabKeyDown}
          role="tablist"
        >
          {TABS.map((tab, i) => (
            <button
              aria-controls={`code-tabpanel-${String(i)}`}
              aria-selected={activeTab === i}
              className={`shrink-0 rounded-full px-3 py-1.5 font-medium text-xs transition-colors sm:px-4 sm:py-2 sm:text-sm ${
                activeTab === i
                  ? "bg-foreground text-background"
                  : "bg-muted text-muted-foreground hover:text-foreground"
              }`}
              id={`code-tab-${String(i)}`}
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

        <Reveal delay={0.1} spring variant="scale">
          <MockBrowserWindow
            actions={<CopyButton text={currentTab.code} />}
            url={currentTab.filename}
          >
            <div
              aria-labelledby={`code-tab-${String(activeTab)}`}
              className="p-5 sm:p-6"
              id={`code-tabpanel-${String(activeTab)}`}
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
                  <SyntaxHighlighter
                    language={currentTab.language}
                    style={customTheme}
                  >
                    {currentTab.code}
                  </SyntaxHighlighter>
                </motion.div>
              </AnimatePresence>
            </div>
          </MockBrowserWindow>
        </Reveal>
      </Shell>
    </section>
  );
};

export default CodeExampleSection;
