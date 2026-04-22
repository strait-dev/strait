import Reveal from "@/components/landing/reveal.tsx";
import {
  StaggerGroup,
  StaggerItem,
} from "@/components/landing/stagger-group.tsx";
import Shell from "@/components/layout/shell.tsx";

const PAIN_POINTS = [
  {
    before: "4 queue systems that disagree on retries",
    after: "One queue with built-in retry strategies",
  },
  {
    before: "Shell scripts coordinating job dependencies",
    after: "Workflow DAGs with conditions and fan-out",
  },
  {
    before: "30 minutes joining logs across 3 services",
    after: "Per-run event timeline with debug bundles",
  },
  {
    before: "Every team reinventing job infrastructure",
    after: "Shared platform with 5 language SDKs",
  },
];

const ProblemSection = () => (
  <section
    aria-label="The problem with background job systems"
    className="bg-muted/30 py-16 sm:py-20"
  >
    <Shell variant="wide">
      <Reveal variant="blur">
        <div className="mb-14 max-w-3xl">
          <h2 className="text-balance text-2xl leading-[1.2] sm:text-3xl lg:text-4xl">
            Stop duct-taping your job infrastructure.
          </h2>
          <p className="mt-3 text-pretty text-muted-foreground text-sm leading-relaxed sm:text-base">
            Every team builds the same glue code -- retries, queues, monitoring.
            Strait replaces all of it.
          </p>
        </div>
      </Reveal>

      <StaggerGroup className="grid grid-cols-1 gap-4 sm:grid-cols-2">
        {PAIN_POINTS.map((point) => (
          <StaggerItem key={point.before}>
            <div className="rounded-xl border border-border/40 bg-card p-5 sm:p-6">
              <p className="text-muted-foreground/50 text-sm leading-relaxed line-through decoration-destructive/40">
                {point.before}
              </p>
              <p className="mt-2 font-medium text-foreground text-sm leading-relaxed">
                <span className="mr-2 inline-block size-1.5 rounded-full bg-primary" />
                {point.after}
              </p>
            </div>
          </StaggerItem>
        ))}
      </StaggerGroup>
    </Shell>
  </section>
);

export default ProblemSection;
