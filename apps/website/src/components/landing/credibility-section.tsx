"use client";

import { useEffect, useRef, useState } from "react";
import Shell from "@/components/layout/shell.tsx";

const ARCHITECTURE = [
  "PostgreSQL SKIP LOCKED queue",
  "13-state run lifecycle FSM",
  "DAG workflow orchestration",
  "Exponential backoff + jitter retries",
  "Dead letter queue with replay",
  "HMAC-SHA256 signed webhooks",
  "Go SDK with heartbeats & checkpoints",
  "Per-run and daily cost budgets",
];

const ArchitectureList = () => {
  const [visibleCount, setVisibleCount] = useState(0);
  const containerRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const el = containerRef.current;
    if (!el) {
      return;
    }

    const observer = new IntersectionObserver(
      ([entry]) => {
        if (entry?.isIntersecting) {
          observer.disconnect();
          let count = 0;
          const id = setInterval(() => {
            count++;
            setVisibleCount(count);
            if (count >= ARCHITECTURE.length) {
              clearInterval(id);
            }
          }, 120);
        }
      },
      { threshold: 0.2 }
    );

    observer.observe(el);
    return () => observer.disconnect();
  }, []);

  return (
    <div className="flex flex-wrap gap-2" ref={containerRef}>
      {ARCHITECTURE.map((item, i) => (
        <span
          className={`rounded-full border border-border/60 bg-card px-3 py-1.5 text-sm transition-all duration-300 ${
            i < visibleCount
              ? "text-foreground opacity-100"
              : "text-muted-foreground/30 opacity-0"
          }`}
          key={item}
        >
          {item}
        </span>
      ))}
    </div>
  );
};

const CredibilitySection = () => (
  <section className="py-20 sm:py-28">
    <Shell variant="wide">
      <div className="mb-14 max-w-3xl">
        <h2 className="text-balance text-2xl leading-[1.2] tracking-tight sm:text-3xl lg:text-4xl">
          <span className="text-foreground">
            Built on proven infrastructure patterns.
          </span>{" "}
          <span className="text-muted-foreground">
            No fake testimonials. Just technical choices you can verify.
          </span>
        </h2>
      </div>

      <div className="grid grid-cols-1 gap-6 lg:grid-cols-3 lg:gap-8">
        {/* Open Source */}
        <div className="rounded-2xl border border-border/60 bg-card p-6 sm:p-8">
          <div className="mb-4 flex items-center gap-3">
            <div className="flex size-10 items-center justify-center rounded-lg bg-muted">
              <svg
                className="size-5 text-foreground"
                fill="currentColor"
                viewBox="0 0 24 24"
              >
                <path d="M12 0c-6.626 0-12 5.373-12 12 0 5.302 3.438 9.8 8.207 11.387.599.111.793-.261.793-.577v-2.234c-3.338.726-4.033-1.416-4.033-1.416-.546-1.387-1.333-1.756-1.333-1.756-1.089-.745.083-.729.083-.729 1.205.084 1.839 1.237 1.839 1.237 1.07 1.834 2.807 1.304 3.492.997.107-.775.418-1.305.762-1.604-2.665-.305-5.467-1.334-5.467-5.931 0-1.311.469-2.381 1.236-3.221-.124-.303-.535-1.524.117-3.176 0 0 1.008-.322 3.301 1.23.957-.266 1.983-.399 3.003-.404 1.02.005 2.047.138 3.006.404 2.291-1.552 3.297-1.23 3.297-1.23.653 1.653.242 2.874.118 3.176.77.84 1.235 1.911 1.235 3.221 0 4.609-2.807 5.624-5.479 5.921.43.372.823 1.102.823 2.222v3.293c0 .319.192.694.801.576 4.765-1.589 8.199-6.086 8.199-11.386 0-6.627-5.373-12-12-12z" />
              </svg>
            </div>
            <div>
              <h3 className="font-semibold text-foreground">Open Source</h3>
              <p className="text-muted-foreground text-sm">Apache 2.0</p>
            </div>
          </div>
          <p className="text-muted-foreground text-sm leading-relaxed">
            Full source on GitHub. Read the queue implementation, audit the FSM
            transitions, or contribute improvements.
          </p>
          <div className="mt-4 flex items-center gap-4">
            <span className="rounded bg-muted px-2 py-0.5 font-mono text-muted-foreground text-xs">
              Go
            </span>
            <span className="text-muted-foreground/60 text-xs">
              Apache 2.0 License
            </span>
          </div>
        </div>

        {/* Technical Foundation */}
        <div className="rounded-2xl border border-border/60 bg-card p-6 sm:p-8">
          <h3 className="mb-4 font-semibold text-foreground">
            Technical Foundation
          </h3>
          <ArchitectureList />
        </div>

        {/* Infrastructure Comparison */}
        <div className="rounded-2xl border border-border/60 bg-card p-6 sm:p-8">
          <h3 className="mb-4 font-semibold text-foreground">
            Infrastructure Comparison
          </h3>
          <div className="space-y-3">
            <div className="rounded-lg bg-muted/30 p-3">
              <p className="mb-2 font-medium text-muted-foreground text-xs uppercase tracking-wider">
                Typical Stack
              </p>
              <div className="flex flex-wrap gap-1.5">
                {["Redis", "Celery/BullMQ", "Airflow", "Custom Retry"].map(
                  (item) => (
                    <span
                      className="rounded bg-destructive/10 px-2 py-0.5 text-destructive text-xs"
                      key={item}
                    >
                      {item}
                    </span>
                  )
                )}
              </div>
              <p className="mt-2 text-muted-foreground/60 text-xs">
                4 services to maintain
              </p>
            </div>
            <div className="rounded-lg bg-success/5 p-3">
              <p className="mb-2 font-medium text-muted-foreground text-xs uppercase tracking-wider">
                Strait
              </p>
              <span className="rounded bg-success/10 px-2 py-0.5 text-success text-xs">
                Single Go binary + Postgres
              </span>
              <p className="mt-2 text-muted-foreground/60 text-xs">
                1 service to maintain
              </p>
            </div>
          </div>
        </div>
      </div>
    </Shell>
  </section>
);

export default CredibilitySection;
