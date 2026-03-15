"use client";

import { ArrowRight02Icon } from "@hugeicons/core-free-icons";
import { HugeiconsIcon } from "@hugeicons/react";
import { Button } from "@strait/ui/components/button";
import Link from "next/link";
import Reveal from "@/components/landing/reveal.tsx";
import Shell from "@/components/layout/shell.tsx";

import { dashboardHref } from "@/lib/urls.ts";

type Step = {
  description: string;
  label: string;
  number: string;
  title: string;
  visual: React.FC;
};

const StepDefine = () => (
  <div className="flex h-full flex-col justify-center gap-3 px-6 py-8">
    <div className="mb-1 flex items-center gap-2">
      <span className="font-medium text-primary-foreground/50 text-xs">
        workflow.ts
      </span>
    </div>

    <div className="space-y-2">
      <div className="flex items-center gap-2">
        <div className="h-2.5 w-[38%] rounded bg-primary-foreground/20" />
        <div className="h-2.5 w-[22%] rounded bg-primary-foreground/15" />
      </div>

      <div className="ml-4 space-y-1.5">
        <div className="flex items-center gap-2">
          <div className="h-2.5 w-[28%] rounded bg-primary-foreground/25" />
          <div className="h-2.5 w-[34%] rounded bg-primary-foreground/15" />
        </div>
        <div className="flex items-center gap-2">
          <div className="h-2.5 w-[20%] rounded bg-primary-foreground/25" />
          <div className="h-2.5 w-[40%] rounded bg-primary-foreground/15" />
        </div>
        <div className="flex items-center gap-2">
          <div className="h-2.5 w-[24%] rounded bg-primary-foreground/25" />
          <div className="h-2.5 w-[30%] rounded bg-primary-foreground/15" />
        </div>
      </div>

      <div className="flex items-center gap-2">
        <div className="h-2.5 w-[14%] rounded bg-primary-foreground/20" />
      </div>

      <div className="mt-1">
        <span className="inline-block h-4 w-0.5 animate-pulse bg-primary-foreground/60" />
      </div>
    </div>
  </div>
);

const StepRun = () => (
  <div className="flex h-full flex-col justify-center gap-4 px-6 py-8">
    <div className="space-y-2">
      <span className="font-medium text-primary-foreground/50 text-xs">
        $ curl
      </span>
      <div className="rounded-lg border border-primary-foreground/10 bg-primary-foreground/8 p-3">
        <div className="space-y-1.5">
          <div className="flex items-center gap-1.5">
            <div className="h-2 w-[18%] rounded bg-primary-foreground/30" />
            <div className="h-2 w-[50%] rounded bg-primary-foreground/15" />
          </div>
          <div className="flex items-center gap-1.5">
            <div className="h-2 w-[12%] rounded bg-primary-foreground/20" />
            <div className="h-2 w-[36%] rounded bg-primary-foreground/15" />
          </div>
        </div>
      </div>
    </div>

    <div className="h-px w-full bg-primary-foreground/10" />

    <div className="space-y-2">
      <span className="font-medium text-primary-foreground/50 text-xs">
        Worker
      </span>
      <div className="rounded-lg border border-primary-foreground/10 bg-primary-foreground/8 p-3">
        <div className="flex items-center gap-2">
          <div className="size-2 rounded-full bg-green-400/70" />
          <span className="text-primary-foreground/70 text-xs">
            Run claimed
          </span>
        </div>
        <div className="mt-2 flex items-center gap-2">
          <div className="h-1.5 flex-1 overflow-hidden rounded-full bg-primary-foreground/10">
            <div className="h-full w-[65%] rounded-full bg-primary-foreground/40" />
          </div>
          <span className="text-primary-foreground/40 text-xs">Step 2/3</span>
        </div>
      </div>
    </div>
  </div>
);

const StepObserve = () => (
  <div className="flex h-full flex-col justify-center gap-4 px-6 py-8">
    <div className="flex items-center justify-between">
      <span className="font-medium text-primary-foreground/50 text-xs">
        Run timeline
      </span>
      <span className="text-primary-foreground/30 text-xs">2m 14s ago</span>
    </div>

    <div className="space-y-2">
      <div className="flex items-center gap-2">
        <div className="h-3 w-[60%] rounded bg-primary-foreground/25" />
        <div className="size-2 rounded-full bg-green-400/70" />
      </div>
      <div className="flex items-center gap-2">
        <div className="h-3 w-[45%] rounded bg-primary-foreground/25" />
        <div className="size-2 rounded-full bg-green-400/70" />
      </div>
      <div className="flex items-center gap-2">
        <div className="h-3 w-[30%] rounded bg-red-400/60" />
        <div className="size-2 rounded-full bg-red-400/60" />
      </div>
    </div>

    <div className="rounded-lg border border-primary-foreground/10 bg-primary-foreground/8 p-3">
      <div className="flex items-center justify-between">
        <div className="space-y-1">
          <span className="block text-primary-foreground/70 text-xs">
            Step 3 failed
          </span>
          <span className="block text-primary-foreground/40 text-xs">
            timeout after 30s
          </span>
        </div>
        <div className="rounded-md bg-primary-foreground/15 px-2.5 py-1">
          <span className="font-medium text-primary-foreground/70 text-xs">
            Replay
          </span>
        </div>
      </div>
    </div>
  </div>
);

const STEPS: Step[] = [
  {
    description:
      "Write your workflow in TypeScript, Go, or Python. Define steps, retries, and timeouts — no YAML, no config files.",
    label: "Define",
    number: "01",
    title: "Define your workflow in code",
    visual: StepDefine,
  },
  {
    description:
      "Trigger a run with a single API call. Workers pick it up instantly and execute each step in order.",
    label: "Run",
    number: "02",
    title: "Trigger and execute runs",
    visual: StepRun,
  },
  {
    description:
      "See every run on a timeline. Inspect failures, read logs, and replay from the exact point of failure.",
    label: "Observe",
    number: "03",
    title: "Observe and recover",
    visual: StepObserve,
  },
];

const HowItWorks = () => {
  const headingId = "how-it-works-title";

  return (
    <section
      aria-labelledby={headingId}
      className="py-20 sm:py-28"
      id="how-it-works"
    >
      <Shell variant="wide">
        <div className="mb-14 max-w-3xl">
          <h2
            className="text-balance text-2xl leading-[1.2] sm:text-3xl lg:text-4xl"
            id={headingId}
          >
            <span className="text-foreground">
              Three steps. No YAML nightmare.
            </span>{" "}
            <span className="text-muted-foreground">
              From first job definition to production observability in under ten
              minutes.
            </span>
          </h2>
        </div>

        <div className="grid grid-cols-1 gap-6 md:grid-cols-2 lg:grid-cols-3 lg:gap-8">
          {STEPS.map((step, idx) => {
            const Visual = step.visual;

            return (
              <Reveal
                className="flex flex-col"
                delay={idx * 0.12}
                key={step.number}
                variant="scale"
                spring
              >
                <div className="relative aspect-[4/3] overflow-hidden rounded-2xl bg-primary">
                  <div className="showcase-dots pointer-events-none absolute inset-0" />
                  <div
                    className="pointer-events-none absolute inset-0 opacity-30"
                    style={{
                      background:
                        "radial-gradient(circle at 50% 40%, oklch(1 0 0 / 0.15), transparent 60%)",
                    }}
                  />
                  <div className="relative z-10 h-full">
                    <Visual />
                  </div>
                </div>
                <div className="mt-5">
                  <div className="flex items-center gap-3">
                    <span className="font-heading text-3xl text-muted-foreground/30 leading-none">
                      {step.number}
                    </span>
                    <div>
                      <span className="block font-medium text-muted-foreground text-xs uppercase">
                        {step.label}
                      </span>
                      <h3 className="font-semibold text-foreground text-lg">
                        {step.title}
                      </h3>
                    </div>
                  </div>
                  <p className="mt-2 text-base text-muted-foreground leading-relaxed">
                    {step.description}
                  </p>
                </div>
              </Reveal>
            );
          })}
        </div>

        <div className="mt-10 flex justify-center">
          <Button render={<Link href={dashboardHref("/login")} />}>
            Create your first workflow
            <HugeiconsIcon className="size-4" icon={ArrowRight02Icon} />
          </Button>
        </div>
      </Shell>
    </section>
  );
};

export default HowItWorks;
