"use client";

import {
  ArrowRight02Icon,
  Cancel01Icon,
  CheckmarkCircle02Icon,
} from "@hugeicons/core-free-icons";
import { HugeiconsIcon } from "@hugeicons/react";
import { Button } from "@strait/ui/components/button";
import Link from "next/link";
import { useEffect, useRef } from "react";

import Shell from "@/components/layout/shell.tsx";
import { dashboardHref } from "@/lib/urls.ts";

const OLD_WAY_STEPS = [
  "Run a separate message broker and keep it healthy",
  "Write custom retry, timeout, and deduplication logic",
  "Build ad-hoc workflow dependency handling",
  "Patch observability across multiple systems",
  "Debug failures by joining logs from different tools",
  "Repeat the same platform work in every project",
];

const STRAIT_STEPS = [
  "Queue runs directly in PostgreSQL with SKIP LOCKED",
  "Use built-in retries, DLQ, and idempotency controls",
  "Model DAG workflows with dependencies and conditions",
  "Add approval gates and nested sub-workflows",
  "Track runs with events, usage, and execution traces",
  "Operate one runtime with API, worker, and CLI support",
];

const ComparisonSection = () => {
  const sectionRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const el = sectionRef.current;
    if (!el) {
      return;
    }

    const observer = new IntersectionObserver(
      ([entry]) => {
        if (entry?.isIntersecting) {
          el.setAttribute("data-visible", "");
          observer.disconnect();
        }
      },
      { threshold: 0.1 }
    );

    observer.observe(el);
    return () => observer.disconnect();
  }, []);

  return (
    <section
      className="border-border/40 border-y py-20 sm:py-28"
      ref={sectionRef}
    >
      <Shell variant="wide">
        <div className="mb-14 max-w-3xl">
          <h2 className="text-balance text-2xl leading-[1.2] tracking-tight sm:text-3xl lg:text-4xl">
            <span className="font-bold text-foreground">
              Replace tool sprawl with one platform your team actually enjoys.
            </span>{" "}
            <span className="text-muted-foreground">
              Strait helps you ship faster and recover quicker without building
              more internal plumbing.
            </span>
          </h2>
        </div>

        <div className="grid grid-cols-1 gap-6 md:grid-cols-2 lg:gap-8">
          <div className="rounded-2xl border border-border/60 bg-card p-6 sm:p-8">
            <div className="mb-6 flex items-center gap-3">
              <span className="flex size-8 items-center justify-center rounded-lg bg-muted text-muted-foreground">
                <HugeiconsIcon className="size-4" icon={Cancel01Icon} />
              </span>
              <h3 className="font-semibold text-foreground text-lg">
                Fragmented stack
              </h3>
            </div>

            <ul className="space-y-3">
              {OLD_WAY_STEPS.map((step) => (
                <li className="flex items-start gap-3" key={step}>
                  <span className="mt-1 flex size-5 shrink-0 items-center justify-center rounded-full bg-muted text-muted-foreground">
                    <HugeiconsIcon className="size-3" icon={Cancel01Icon} />
                  </span>
                  <span className="text-muted-foreground text-sm leading-relaxed">
                    {step}
                  </span>
                </li>
              ))}
            </ul>
          </div>

          <div className="rounded-2xl border border-foreground/10 bg-card p-6 sm:p-8">
            <div className="mb-6 flex items-center gap-3">
              <span className="icon-chip text-foreground">
                <HugeiconsIcon
                  className="size-4"
                  icon={CheckmarkCircle02Icon}
                />
              </span>
              <h3 className="font-semibold text-foreground text-lg">
                Strait runtime
              </h3>
            </div>

            <ul className="space-y-3">
              {STRAIT_STEPS.map((step) => (
                <li className="flex items-start gap-3" key={step}>
                  <span className="mt-1 flex size-5 shrink-0 items-center justify-center rounded-full bg-muted text-foreground">
                    <HugeiconsIcon
                      className="size-3"
                      icon={CheckmarkCircle02Icon}
                    />
                  </span>
                  <span className="text-foreground text-sm leading-relaxed">
                    {step}
                  </span>
                </li>
              ))}
            </ul>
          </div>
        </div>

        <div className="mt-10 flex justify-center">
          <Button
            className="bg-primary text-primary-foreground transition-all duration-300 hover:bg-primary/90"
            render={<Link href={dashboardHref("/login")} />}
            size="lg"
          >
            See Strait in action
            <HugeiconsIcon className="size-4" icon={ArrowRight02Icon} />
          </Button>
        </div>
      </Shell>
    </section>
  );
};

export default ComparisonSection;
