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
    subtitle: "Never lose a job again",
    description:
      "Every run tracks through 13 states from queued to completed. Failed jobs retry with exponential backoff, dead runs route to DLQ for review, and stale jobs clean up on their own.",
    span: "sm:col-span-1 lg:col-span-6",
    Animation: QueueAnimation,
  },
  {
    id: "workflow-dags",
    title: "Workflow DAGs",
    subtitle: "Complex pipelines, simple code",
    description:
      "Wire steps into dependency graphs with conditions, approval gates, and fan-out/fan-in. Stop coordinating jobs with shell scripts and cron hacks.",
    span: "sm:col-span-1 lg:col-span-8",
    Animation: HeroDag,
  },
  {
    id: "managed-execution",
    title: "Managed Execution",
    subtitle: "Zero infrastructure to manage",
    description:
      "Your code runs in containers with warm pools, multi-region deployment, and automatic scaling. Ship without provisioning servers.",
    span: "sm:col-span-1 lg:col-span-4",
    Animation: ExecutionAnimation,
  },
  {
    id: "ai-agent-platform",
    title: "AI Agent Platform",
    subtitle: "Run agents without runaway costs",
    description:
      "Set per-run cost budgets, require human approval before expensive operations, and track token usage across every model call. Stay in control as agents scale.",
    span: "sm:col-span-1 lg:col-span-6",
    Animation: CostAnimation,
  },
  {
    id: "language-sdks",
    title: "5 Language SDKs",
    subtitle: "Use the language you already know",
    description:
      "TypeScript, Python, Go, Ruby, and Rust. Full coverage with logging, heartbeats, checkpoints, and continuation built into every SDK.",
    span: "sm:col-span-1 lg:col-span-6",
    Animation: SdkAnimation,
  },
  {
    id: "observability",
    title: "Built-in Observability",
    subtitle: "Debug in seconds, not hours",
    description:
      "When a job fails, see exactly why. Health scores combine queue depth, throughput, and latency into a single number. OpenTelemetry and structured logs included.",
    span: "sm:col-span-1 lg:col-span-6",
    Animation: HealthAnimation,
  },
];

/* -- Main component -- */
const FeatureBentoGrid = () => (
  <section className="py-20 sm:py-28" id="features">
    <Shell variant="wide">
      <div className="mb-14 max-w-3xl">
        <h2 className="text-balance text-2xl leading-[1.2] sm:text-3xl lg:text-4xl">
          Write code as if failures don&apos;t exist.
        </h2>
        <p className="mt-3 text-pretty text-muted-foreground text-sm leading-relaxed sm:text-base">
          Strait handles retries, state, scaling, and observability so you can
          focus on your product.
        </p>
      </div>

      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-12">
        {FEATURES.map((feature, idx) => (
          <Reveal
            className={`group overflow-hidden rounded-2xl border border-border/40 bg-card/50 shadow-sm transition-[border-color,box-shadow] duration-300 hover:border-border/60 hover:shadow-lg ${feature.span}`}
            delay={idx * 0.06}
            key={feature.id}
            spring
            variant={idx % 2 === 0 ? "fade-up" : "scale"}
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
              <p className="text-muted-foreground text-xs uppercase">
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
