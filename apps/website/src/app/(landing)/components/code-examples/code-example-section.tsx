"use client";

import { useState } from "react";
import Reveal from "@/components/landing/reveal.tsx";
import Shell from "@/components/layout/shell.tsx";
import MockBrowserWindow from "@/components/magicui/mock-browser-window.tsx";

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

  return (
    <section className="py-20 sm:py-28">
      <Shell variant="wide">
        <Reveal variant="blur">
          <div className="mb-14 max-w-3xl">
            <h2 className="text-balance text-2xl leading-[1.2] sm:text-3xl lg:text-4xl">
              <span className="text-foreground">See how it works.</span>{" "}
              <span className="text-muted-foreground">
                Define jobs, build workflows, set AI guardrails, and stream
                status to your frontend.
              </span>
            </h2>
          </div>
        </Reveal>

        {/* Tab bar */}
        <div className="mb-6 flex flex-wrap items-center gap-2">
          {TABS.map((tab, i) => (
            <button
              className={`rounded-full px-4 py-2 font-medium text-sm transition-colors ${
                activeTab === i
                  ? "bg-foreground text-background"
                  : "bg-muted text-muted-foreground hover:text-foreground"
              }`}
              key={tab.label}
              onClick={() => setActiveTab(i)}
              type="button"
            >
              {tab.label}
            </button>
          ))}
        </div>

        <Reveal delay={0.1} spring variant="scale">
          <MockBrowserWindow url={currentTab.filename}>
            <div className="p-5 sm:p-6">
              <CodeDisplay tokens={currentTab.lines} />
            </div>
          </MockBrowserWindow>
        </Reveal>
      </Shell>
    </section>
  );
};

export default CodeExampleSection;
