import HeroDag from "@/components/landing/hero-dag.tsx";
import Reveal from "@/components/landing/reveal.tsx";
import Shell from "@/components/layout/shell.tsx";
import {
  CostAnimation,
  ExecutionAnimation,
  HealthAnimation,
  QueueAnimation,
  SdkAnimation,
} from "./feature-animations.tsx";

/* -- Feature card data -- */
type FeatureCard = {
  id: string;
  title: string;
  subtitle: string;
  description: string;
  span: string;
  Animation: React.FC;
};

const FEATURES: FeatureCard[] = [
  {
    id: "job-orchestration",
    title: "Job Orchestration",
    subtitle: "Every job finishes or explains why",
    description:
      "Every job runs to completion or tells you why it didn't. Failed jobs retry automatically, unrecoverable failures surface for review, and abandoned jobs clean up on their own.",
    span: "sm:col-span-1 lg:col-span-4",
    Animation: QueueAnimation,
  },
  {
    id: "workflow-dags",
    title: "Workflow DAGs",
    subtitle: "Multi-step pipelines in a few lines",
    description:
      "Chain jobs into multi-step workflows with dependencies, conditions, approval gates, and parallel execution. Define complex pipelines in a few lines of code.",
    span: "sm:col-span-1 lg:col-span-8",
    Animation: HeroDag,
  },
  {
    id: "managed-execution",
    title: "Managed Execution",
    subtitle: "Your code runs, you don't manage servers",
    description:
      "Your code runs in containers with warm pools, multi-region deployment, and automatic scaling. Ship without provisioning servers.",
    span: "sm:col-span-1 lg:col-span-6",
    Animation: ExecutionAnimation,
  },
  {
    id: "ai-agent-platform",
    title: "AI Agent Platform",
    subtitle: "AI agents that stay on budget",
    description:
      "Set per-run cost budgets, require human approval before expensive operations, and track token usage across every model call. Stay in control as agents scale.",
    span: "sm:col-span-1 lg:col-span-6",
    Animation: CostAnimation,
  },
  {
    id: "language-sdks",
    title: "5 Language SDKs",
    subtitle: "Works with your existing stack",
    description:
      "TypeScript, Python, Go, Ruby, and Rust. Every SDK includes logging, progress reporting, checkpoints, and long-running job support.",
    span: "sm:col-span-1 lg:col-span-6",
    Animation: SdkAnimation,
  },
  {
    id: "observability",
    title: "Built-in Observability",
    subtitle: "Debug in seconds, not hours",
    description:
      "When a job fails, see exactly what went wrong. Health scores give you a single number for system health. OpenTelemetry tracing and structured logs included.",
    span: "sm:col-span-1 lg:col-span-6",
    Animation: HealthAnimation,
  },
];

/* -- Main component -- */
const FeatureBentoGrid = () => (
  <section
    className="infinity-border-y overflow-hidden py-20 sm:py-28"
    id="features"
  >
    <Shell variant="wide">
      <div className="mb-14 max-w-3xl">
        <h2 className="text-balance text-2xl leading-[1.2] sm:text-3xl lg:text-4xl">
          Write code as if{" "}
          <span className="text-primary">failures don&apos;t exist</span>.
        </h2>
        <p className="mt-3 text-pretty text-muted-foreground text-sm leading-relaxed sm:text-base">
          Focus on what your code does. Strait handles everything that happens
          after you hit run.
        </p>
      </div>

      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-12">
        {FEATURES.map((feature, idx) => (
          <Reveal
            className={`group overflow-hidden rounded-2xl border border-border/40 bg-card/50 shadow-sm transition-[border-color,box-shadow] duration-150 hover:border-border/60 hover:shadow-lg ${feature.span}`}
            delay={idx * 0.06}
            key={feature.id}
            spring
            variant="fade-up"
          >
            {/* Animation area */}
            <div className="relative flex h-48 items-center justify-center overflow-hidden bg-muted/20 p-6">
              <div className="showcase-dots pointer-events-none absolute inset-0 opacity-30" />
              <div className="relative z-10">
                <feature.Animation />
              </div>
            </div>

            {/* Content */}
            <div className="p-5 sm:p-6">
              <p className="flex items-center gap-1.5 text-muted-foreground text-xs uppercase">
                <span className="inline-block size-1.5 rounded-full bg-primary" />
                {feature.subtitle}
              </p>
              <h3 className="mt-1 font-semibold text-foreground text-lg">
                {feature.title}
              </h3>
              <p className="mt-2 text-muted-foreground text-sm leading-relaxed">
                {feature.description}
              </p>
            </div>
          </Reveal>
        ))}
      </div>
    </Shell>
  </section>
);

export default FeatureBentoGrid;
