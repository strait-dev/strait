"use client";

import { ArrowRight02Icon } from "@hugeicons/core-free-icons";
import { HugeiconsIcon } from "@hugeicons/react";
import { Button } from "@strait/ui/components/button";
import Link from "next/link";
import { useState } from "react";

import Shell from "@/components/layout/shell.tsx";
import MockBrowserWindow from "@/components/magicui/mock-browser-window.tsx";
import TerminalAnimation from "@/components/magicui/terminal-animation.tsx";
import { dashboardHref } from "@/lib/urls.ts";

const WITHOUT_CODE = `// redis-queue.js
const queue = new Bull('process-order');
queue.process(async (job) => {
  // custom retry logic...
  // manual error handling...
  // ad-hoc workflow state...
  // scattered observability...
});

// cron-service.js
cron.schedule('*/5 * * * *', checkStuckJobs);

// webhook-handler.js
app.post('/webhook', customRetryWrapper(handler));

// monitoring.js
setupCustomMetrics(prometheus, redis, postgres);`;

const WITH_CODE = `// workflow.ts
await strait.jobs.create({
  name: "process-order",
  workflowId: "checkout-flow",
  retries: 3,
  backoff: "exponential",
  budget: "$12/run",
})

// That's it. Retries, DLQ, events,
// health scoring, and streaming are built in.`;

const METRICS = [
  { label: "Lines of infra code", without: 2400, with: 120, unit: "" },
  { label: "Systems to monitor", without: 5, with: 1, unit: "" },
  { label: "MTTR", without: 45, with: 3, unit: "min" },
];

const ComparisonSection = () => {
  const [showStrait, setShowStrait] = useState(false);

  return (
    <section className="border-border/40 border-y py-20 sm:py-28">
      <Shell variant="wide">
        <div className="mb-14 max-w-3xl">
          <h2 className="text-balance text-2xl leading-[1.2] sm:text-3xl lg:text-4xl">
            <span className="text-foreground">
              2,400 lines of glue vs. 10 lines of Strait.
            </span>{" "}
            <span className="text-muted-foreground">
              Toggle to see what your infra looks like before and after.
            </span>
          </h2>
        </div>

        {/* Toggle */}
        <div className="mb-8 flex items-center justify-center gap-2">
          <button
            className={`rounded-full px-4 py-2 font-medium text-sm transition-colors ${
              showStrait
                ? "bg-muted text-muted-foreground"
                : "bg-foreground text-background"
            }`}
            onClick={() => setShowStrait(false)}
            type="button"
          >
            Without Strait
          </button>
          <button
            className={`rounded-full px-4 py-2 font-medium text-sm transition-colors ${
              showStrait
                ? "bg-foreground text-background"
                : "bg-muted text-muted-foreground"
            }`}
            onClick={() => setShowStrait(true)}
            type="button"
          >
            With Strait
          </button>
        </div>

        {/* Code comparison */}
        <MockBrowserWindow url={showStrait ? "workflow.ts" : "infrastructure/"}>
          {showStrait ? (
            <div className="p-6">
              <TerminalAnimation
                className="text-success/80 leading-relaxed"
                code={WITH_CODE}
                typingSpeed={25}
              />
            </div>
          ) : (
            <pre className="overflow-x-auto p-6 font-mono text-sm leading-relaxed">
              <code className="text-muted-foreground/70">{WITHOUT_CODE}</code>
            </pre>
          )}
        </MockBrowserWindow>

        {/* Metrics */}
        <div className="mt-8 grid grid-cols-1 gap-4 sm:grid-cols-3">
          {METRICS.map((metric) => {
            const value = showStrait ? metric.with : metric.without;
            return (
              <div
                className="rounded-xl border border-border/60 bg-card p-5 text-center"
                key={metric.label}
              >
                <p className="font-semibold text-3xl text-foreground tabular-nums transition-opacity duration-300">
                  {value}
                  {metric.unit}
                </p>
                <p className="mt-1 text-muted-foreground text-sm">
                  {metric.label}
                </p>
              </div>
            );
          })}
        </div>

        <div className="mt-10 flex justify-center">
          <Button render={<Link href={dashboardHref("/login")} />}>
            Try it free
            <HugeiconsIcon className="size-4" icon={ArrowRight02Icon} />
          </Button>
        </div>
      </Shell>
    </section>
  );
};

export default ComparisonSection;
