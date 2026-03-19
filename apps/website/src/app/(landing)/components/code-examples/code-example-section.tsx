"use client";

import { AnimatePresence, motion } from "motion/react";
import { useCallback, useState } from "react";
import Reveal from "@/components/landing/reveal.tsx";
import Shell from "@/components/layout/shell.tsx";
import MockBrowserWindow from "@/components/magicui/mock-browser-window.tsx";
import { SPRING_SNAPPY } from "@/lib/motion.ts";

const TABS = [
  {
    label: "Define a job",
    filename: "jobs/process-order.ts",
    lines: [
      { text: "import", kind: "keyword" },
      { text: " { defineJob } ", kind: "default" },
      { text: "from", kind: "keyword" },
      { text: ' "@strait/sdk"', kind: "string" },
      { text: ";\n\n", kind: "default" },
      { text: "export default", kind: "keyword" },
      { text: " defineJob(", kind: "default" },
      { text: '"process-order"', kind: "string" },
      { text: ", {\n", kind: "default" },
      { text: "  retries: 3,\n", kind: "default" },
      { text: "  backoff: ", kind: "default" },
      { text: '"exponential"', kind: "string" },
      { text: ",\n", kind: "default" },
      { text: "  timeout: ", kind: "default" },
      { text: '"30s"', kind: "string" },
      { text: ",\n\n", kind: "default" },
      { text: "  async", kind: "keyword" },
      { text: " handler(run) {\n", kind: "default" },
      { text: "    ", kind: "default" },
      { text: "const", kind: "keyword" },
      { text: " order = ", kind: "default" },
      { text: "await", kind: "keyword" },
      { text: " db.orders.find(run.payload.orderId);\n", kind: "default" },
      { text: "    ", kind: "default" },
      { text: "await", kind: "keyword" },
      { text: " chargePayment(order);\n", kind: "default" },
      { text: "    ", kind: "default" },
      { text: "await", kind: "keyword" },
      { text: " sendConfirmation(order);\n", kind: "default" },
      { text: "  },\n});", kind: "default" },
    ],
  },
  {
    label: "Create a workflow",
    filename: "workflows/checkout.ts",
    lines: [
      { text: "import", kind: "keyword" },
      { text: " { defineWorkflow } ", kind: "default" },
      { text: "from", kind: "keyword" },
      { text: ' "@strait/sdk"', kind: "string" },
      { text: ";\n\n", kind: "default" },
      { text: "export default", kind: "keyword" },
      { text: " defineWorkflow(", kind: "default" },
      { text: '"checkout-flow"', kind: "string" },
      { text: ", {\n", kind: "default" },
      { text: "  steps: {\n", kind: "default" },
      { text: "    validate:  { job: ", kind: "default" },
      { text: '"validate-order"', kind: "string" },
      { text: " },\n", kind: "default" },
      { text: "    charge:   { job: ", kind: "default" },
      { text: '"charge-payment"', kind: "string" },
      { text: ", after: [", kind: "default" },
      { text: '"validate"', kind: "string" },
      { text: "] },\n", kind: "default" },
      { text: "    approve:  { gate: ", kind: "default" },
      { text: '"manual"', kind: "string" },
      { text: ", after: [", kind: "default" },
      { text: '"charge"', kind: "string" },
      { text: "] },\n", kind: "default" },
      { text: "    fulfill:  { job: ", kind: "default" },
      { text: '"fulfill-order"', kind: "string" },
      { text: ", after: [", kind: "default" },
      { text: '"approve"', kind: "string" },
      { text: "] },\n", kind: "default" },
      { text: "    notify:   { job: ", kind: "default" },
      { text: '"send-receipt"', kind: "string" },
      { text: ", after: [", kind: "default" },
      { text: '"fulfill"', kind: "string" },
      { text: "] },\n", kind: "default" },
      { text: "  },\n});", kind: "default" },
    ],
  },
  {
    label: "AI agent guardrails",
    filename: "jobs/ai-research.ts",
    lines: [
      { text: "import", kind: "keyword" },
      { text: " { defineJob } ", kind: "default" },
      { text: "from", kind: "keyword" },
      { text: ' "@strait/sdk"', kind: "string" },
      { text: ";\n\n", kind: "default" },
      { text: "export default", kind: "keyword" },
      { text: " defineJob(", kind: "default" },
      { text: '"ai-research-agent"', kind: "string" },
      { text: ", {\n", kind: "default" },
      { text: "  budget: { maxCost: ", kind: "default" },
      { text: '"$5.00"', kind: "string" },
      { text: ", model: ", kind: "default" },
      { text: '"gpt-4o"', kind: "string" },
      { text: " },\n", kind: "default" },
      { text: "  approvalRequired: ", kind: "default" },
      { text: "true", kind: "keyword" },
      { text: ",\n", kind: "default" },
      { text: "  retries: 1,\n\n", kind: "default" },
      { text: "  async", kind: "keyword" },
      { text: " handler(run) {\n", kind: "default" },
      { text: "    ", kind: "default" },
      { text: "const", kind: "keyword" },
      { text: " result = ", kind: "default" },
      { text: "await", kind: "keyword" },
      { text: " run.ai.complete({\n", kind: "default" },
      { text: "      prompt: run.payload.query,\n", kind: "default" },
      { text: "      tools: [searchWeb, readDocs],\n", kind: "default" },
      { text: "    });\n", kind: "default" },
      { text: "    ", kind: "default" },
      { text: "return", kind: "keyword" },
      { text: " result;\n", kind: "default" },
      { text: "    ", kind: "default" },
      { text: "// cost is tracked automatically", kind: "comment" },
      { text: "\n  },\n});", kind: "default" },
    ],
  },
  {
    label: "Stream with React",
    filename: "components/order-status.tsx",
    lines: [
      { text: "import", kind: "keyword" },
      { text: " { useRun } ", kind: "default" },
      { text: "from", kind: "keyword" },
      { text: ' "@strait/react"', kind: "string" },
      { text: ";\n\n", kind: "default" },
      { text: "export function", kind: "keyword" },
      { text: " OrderStatus({ runId }: { runId: ", kind: "default" },
      { text: "string", kind: "keyword" },
      { text: " }) {\n", kind: "default" },
      { text: "  ", kind: "default" },
      { text: "const", kind: "keyword" },
      {
        text: " { status, steps, cost } = useRun(runId);\n\n",
        kind: "default",
      },
      { text: "  ", kind: "default" },
      { text: "return", kind: "keyword" },
      { text: " (\n", kind: "default" },
      { text: "    <div>\n", kind: "default" },
      { text: "      <p>Status: {status}</p>\n", kind: "default" },
      // biome-ignore lint/suspicious/noTemplateCurlyInString: intentional code display text
      { text: "      <p>Cost: ${cost.total}</p>\n", kind: "default" },
      { text: "      {steps.map(step =>\n", kind: "default" },
      { text: "        <Step ", kind: "default" },
      { text: "key", kind: "keyword" },
      { text: "={step.id} {...step} />\n", kind: "default" },
      { text: "      )}\n", kind: "default" },
      { text: "    </div>\n  );\n}", kind: "default" },
    ],
  },
] as const;

type CodeToken = { text: string; kind: string };

function getPlainText(tokens: readonly CodeToken[]): string {
  return tokens.map((t) => t.text).join("");
}

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

const CodeDisplay = ({ tokens }: { tokens: readonly CodeToken[] }) => (
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
    <section className="py-20 sm:py-28">
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
            actions={<CopyButton text={getPlainText(currentTab.lines)} />}
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
                  <CodeDisplay tokens={currentTab.lines} />
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
