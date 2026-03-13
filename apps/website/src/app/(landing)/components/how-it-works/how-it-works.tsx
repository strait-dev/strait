import {
  ArrowRight02Icon,
  Chatting01Icon,
  FileEditIcon,
  SparklesIcon,
} from "@hugeicons/core-free-icons";
import { HugeiconsIcon } from "@hugeicons/react";
import { Button } from "@strait/ui/components/button";
import Link from "next/link";
import Shell from "@/components/layout/shell.tsx";

import { dashboardHref } from "@/lib/urls.ts";

const ICON_MAP: Record<string, typeof Chatting01Icon> = {
  "chatting-01": Chatting01Icon,
  sparkles: SparklesIcon,
  "file-edit": FileEditIcon,
};

type StepItem = {
  _id: string;
  title: string;
  description: string;
  icon_name: string;
};

const STEPS: StepItem[] = [
  {
    _id: "step-1",
    title: "Set up your workflow",
    description:
      "Define what should run, when it should run, and how failures should be handled before they become incidents.",
    icon_name: "chatting-01",
  },
  {
    _id: "step-2",
    title: "Launch work with confidence",
    description:
      "Trigger runs from your app or CLI and let Strait move each step forward in the right order.",
    icon_name: "sparkles",
  },
  {
    _id: "step-3",
    title: "Fix issues fast",
    description:
      "See exactly what happened, replay failed runs, and get work back on track without rebuilding your pipeline.",
    icon_name: "file-edit",
  },
];

const StepVisual0 = () => (
  <div className="flex h-full flex-col justify-center gap-3 px-6 py-8">
    <div className="mb-1 flex items-center gap-2">
      <div className="flex size-7 items-center justify-center rounded-lg bg-primary-foreground/20">
        <svg
          className="size-3.5 text-primary-foreground/70"
          fill="none"
          stroke="currentColor"
          strokeWidth={2}
          viewBox="0 0 24 24"
        >
          <path
            d="M12 20h9M16.5 3.5a2.121 2.121 0 013 3L7 19l-4 1 1-4L16.5 3.5z"
            strokeLinecap="round"
            strokeLinejoin="round"
          />
        </svg>
      </div>
      <span className="font-medium text-primary-foreground/50 text-xs">
        Job definition
      </span>
    </div>

    <div className="space-y-2.5">
      <div className="h-3.5 w-[60%] rounded bg-primary-foreground/25" />
      <div className="h-2.5 w-full rounded bg-primary-foreground/15" />
      <div className="h-2.5 w-[92%] rounded bg-primary-foreground/15" />
      <div className="h-2.5 w-[78%] rounded bg-primary-foreground/15" />
    </div>

    <div className="mt-1">
      <span className="inline-block h-4 w-0.5 animate-pulse bg-primary-foreground/60" />
    </div>
  </div>
);

const StepVisual1 = () => (
  <div className="flex h-full flex-col justify-center gap-3 px-6 py-8">
    <div className="flex justify-end">
      <div className="rounded-2xl rounded-br-sm bg-primary-foreground/20 px-4 py-2.5">
        <p className="text-primary-foreground/90 text-sm">
          Trigger workflow run
        </p>
      </div>
    </div>

    <div className="flex justify-start">
      <div className="max-w-[85%] rounded-2xl rounded-bl-sm border border-primary-foreground/15 bg-primary-foreground/10 px-4 py-2.5">
        <p className="text-primary-foreground/80 text-sm leading-relaxed">
          Workflow is running. Step 1 completed. Waiting for approval gate on
          step 2.
        </p>
      </div>
    </div>

    <div className="flex justify-start">
      <div className="flex gap-1.5 rounded-2xl rounded-bl-sm border border-primary-foreground/15 bg-primary-foreground/10 px-4 py-3">
        <span className="size-1.5 animate-pulse rounded-full bg-primary-foreground/50" />
        <span
          className="size-1.5 animate-pulse rounded-full bg-primary-foreground/50"
          style={{ animationDelay: "0.15s" }}
        />
        <span
          className="size-1.5 animate-pulse rounded-full bg-primary-foreground/50"
          style={{ animationDelay: "0.3s" }}
        />
      </div>
    </div>
  </div>
);

const StepVisual2 = () => (
  <div className="flex h-full flex-col justify-center gap-4 px-6 py-8">
    <div className="flex items-center gap-2">
      <svg
        className="size-4 text-primary-foreground/60"
        fill="none"
        stroke="currentColor"
        strokeWidth={2}
        viewBox="0 0 24 24"
      >
        <path
          d="M14 2H6a2 2 0 00-2 2v16a2 2 0 002 2h12a2 2 0 002-2V8z"
          strokeLinecap="round"
          strokeLinejoin="round"
        />
        <path
          d="M14 2v6h6M16 13H8M16 17H8M10 9H8"
          strokeLinecap="round"
          strokeLinejoin="round"
        />
      </svg>
      <span className="font-medium text-primary-foreground/50 text-xs">
        Run operations
      </span>
    </div>

    <div className="space-y-2">
      {["Events", "Debug Bundle", "Replay"].map((action) => (
        <div
          className="flex items-center justify-between rounded-lg border border-primary-foreground/10 bg-primary-foreground/8 px-3 py-2"
          key={action}
        >
          <span className="text-primary-foreground/70 text-xs">{action}</span>
          <svg
            className="size-3.5 text-primary-foreground/40"
            fill="none"
            stroke="currentColor"
            strokeWidth={2}
            viewBox="0 0 24 24"
          >
            <path
              d="M21 15v4a2 2 0 01-2 2H5a2 2 0 01-2-2v-4M7 10l5 5 5-5M12 15V3"
              strokeLinecap="round"
              strokeLinejoin="round"
            />
          </svg>
        </div>
      ))}
    </div>

    <div className="flex items-center gap-2">
      <div className="h-1.5 flex-1 overflow-hidden rounded-full bg-primary-foreground/10">
        <div className="h-full w-[75%] rounded-full bg-primary-foreground/40" />
      </div>
      <span className="text-primary-foreground/40 text-xs">Healthy</span>
    </div>
  </div>
);

const STEP_VISUALS: Record<number, React.FC> = {
  0: StepVisual0,
  1: StepVisual1,
  2: StepVisual2,
};

const HowItWorks = () => {
  const sectionTitle = "From idea to completed run in a few clear steps";
  const sectionDescription =
    "Set up your flow once, then let your team launch, monitor, and recover work from one dashboard.";
  const ctaText = "Create your first workflow";
  const ctaHref = "/login";

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
            className="text-balance text-2xl leading-[1.2] tracking-tight sm:text-3xl lg:text-4xl"
            id={headingId}
          >
            <span className="text-foreground">{sectionTitle}</span>
            {sectionDescription ? (
              <>
                {" "}
                <span className="text-muted-foreground">
                  {sectionDescription}
                </span>
              </>
            ) : null}
          </h2>
        </div>

        <div className="grid grid-cols-1 gap-6 md:grid-cols-2 lg:grid-cols-3 lg:gap-8">
          {STEPS.map((step, index) => {
            const IconComponent = ICON_MAP[step.icon_name] ?? Chatting01Icon;
            const StepVisual = STEP_VISUALS[index];

            return (
              <div className="flex flex-col" key={step._id}>
                <div className="relative aspect-square overflow-hidden rounded-2xl bg-primary">
                  <div className="showcase-dots pointer-events-none absolute inset-0" />
                  <div
                    className="pointer-events-none absolute inset-0 opacity-30"
                    style={{
                      background:
                        "radial-gradient(circle at 50% 40%, oklch(1 0 0 / 0.15), transparent 60%)",
                    }}
                  />
                  <div className="relative z-10 h-full">
                    {StepVisual ? (
                      <StepVisual />
                    ) : (
                      <div className="flex h-full items-center justify-center">
                        <HugeiconsIcon
                          className="size-10 text-primary-foreground/40"
                          icon={IconComponent}
                        />
                      </div>
                    )}
                  </div>
                </div>
                <div className="mt-5">
                  <div className="flex items-center gap-3">
                    <div className="icon-chip">
                      <HugeiconsIcon
                        className="size-4 text-foreground"
                        icon={IconComponent}
                      />
                    </div>
                    <h3 className="font-semibold text-foreground text-lg">
                      {step.title}
                    </h3>
                  </div>
                  <p className="mt-2 text-base text-muted-foreground leading-relaxed">
                    {step.description}
                  </p>
                </div>
              </div>
            );
          })}
        </div>

        <div className="mt-10 flex justify-center">
          <Button render={<Link href={dashboardHref(ctaHref)} />}>
            {ctaText}
            <HugeiconsIcon className="size-4" icon={ArrowRight02Icon} />
          </Button>
        </div>
      </Shell>
    </section>
  );
};

export default HowItWorks;
