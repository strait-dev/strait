export type ComparisonCategory = {
  name: string;
  features: Array<{
    feature: string;
    strait: string | boolean;
    competitor: string | boolean;
  }>;
};

export type ComparisonPageData = {
  slug: string;
  competitor: string;
  tagline: string;
  description: string;
  differentiators: Array<{
    title: string;
    strait: string;
    competitor: string;
  }>;
  categories: ComparisonCategory[];
  switchingSteps: string[];
};

const comparisons: ComparisonPageData[] = [
  {
    slug: "temporal",
    competitor: "Temporal",
    tagline: "Simpler ops — just Postgres, no cluster",
    description:
      "Temporal is powerful but demands a dedicated cluster with Cassandra or MySQL, plus Elasticsearch for visibility. Strait delivers durable execution with just Postgres — no extra infrastructure to provision, scale, or monitor.",
    differentiators: [
      {
        title: "Infrastructure footprint",
        strait: "Single Postgres instance with Postgres-backed queuing",
        competitor:
          "Requires a multi-node cluster with Cassandra/MySQL and Elasticsearch",
      },
      {
        title: "Operational complexity",
        strait:
          "Zero-config queue backed by your existing database — no cluster management",
        competitor:
          "Dedicated ops team recommended for cluster upgrades, schema migrations, and scaling",
      },
      {
        title: "Workflow modeling",
        strait:
          "Declarative DAGs with step conditions, approval gates, and cost budgets",
        competitor:
          "Imperative code-based workflows using language-specific SDKs",
      },
    ],
    categories: [
      {
        name: "Infrastructure & Operations",
        features: [
          {
            feature: "Runs on Postgres only",
            strait: true,
            competitor: false,
          },
          {
            feature: "No external cluster required",
            strait: true,
            competitor: false,
          },
          {
            feature: "Built-in visibility dashboard",
            strait: true,
            competitor: "Requires Elasticsearch add-on",
          },
          {
            feature: "Sub-minute setup",
            strait: true,
            competitor: false,
          },
        ],
      },
      {
        name: "Workflow Capabilities",
        features: [
          {
            feature: "DAG workflow orchestration",
            strait: true,
            competitor: true,
          },
          {
            feature: "Human-in-the-loop approvals",
            strait: true,
            competitor: false,
          },
          {
            feature: "Cost budget enforcement",
            strait: true,
            competitor: false,
          },
          {
            feature: "Retry strategies with jitter",
            strait: true,
            competitor: true,
          },
          {
            feature: "Dead letter queue and replay",
            strait: true,
            competitor: true,
          },
        ],
      },
    ],
    switchingSteps: [
      "Export your Temporal workflow definitions and map them to Strait DAG steps with conditions and approval gates.",
      "Point Strait at your existing Postgres database — no Cassandra, Elasticsearch, or cluster provisioning needed.",
      "Deploy workers using the Strait SDK and validate runs through the built-in observability dashboard.",
      "Decommission your Temporal cluster and reclaim infrastructure spend.",
    ],
  },
  {
    slug: "inngest",
    competitor: "Inngest",
    tagline: "Self-hosted, no vendor lock-in, multi-language SDKs",
    description:
      "Inngest offers a polished developer experience but locks you into their managed cloud. Strait gives you full control — self-host on your own infrastructure, own your data, and build workflows in Go without vendor dependencies.",
    differentiators: [
      {
        title: "Deployment model",
        strait:
          "Fully self-hosted on your infrastructure — run anywhere Docker runs",
        competitor: "Cloud-hosted SaaS with limited self-hosting options",
      },
      {
        title: "Language ecosystem",
        strait: "TypeScript, Go & Python SDKs with first-class concurrency and type safety",
        competitor: "TypeScript-first with limited Go support",
      },
      {
        title: "Data ownership",
        strait:
          "All data lives in your Postgres — full control, no external dependencies",
        competitor: "Run data stored in vendor-managed infrastructure",
      },
    ],
    categories: [
      {
        name: "Hosting & Control",
        features: [
          {
            feature: "Fully self-hosted",
            strait: true,
            competitor: false,
          },
          {
            feature: "No vendor lock-in",
            strait: true,
            competitor: false,
          },
          {
            feature: "Data stays in your database",
            strait: true,
            competitor: false,
          },
          {
            feature: "Open-source core",
            strait: true,
            competitor: "Partial",
          },
        ],
      },
      {
        name: "Developer Experience",
        features: [
          {
            feature: "Multi-language SDKs (TS, Go, Python)",
            strait: true,
            competitor: "Limited",
          },
          {
            feature: "Declarative DAG workflows",
            strait: true,
            competitor: true,
          },
          {
            feature: "Built-in CLI and TUI",
            strait: true,
            competitor: false,
          },
          {
            feature: "Step-level observability",
            strait: true,
            competitor: true,
          },
          {
            feature: "Cost budget controls",
            strait: true,
            competitor: false,
          },
        ],
      },
    ],
    switchingSteps: [
      "Map your Inngest functions to Strait workflow DAGs with step conditions and retry policies.",
      "Deploy Strait on your infrastructure and connect it to your Postgres database.",
      "Migrate event triggers to Strait's API-driven or schedule-based invocation model.",
    ],
  },
  {
    slug: "bullmq",
    competitor: "BullMQ",
    tagline: "No Redis dependency, DAG workflows, approvals",
    description:
      "BullMQ is a solid Redis-based queue for Node.js, but it stops at simple job processing. Strait replaces Redis with Postgres, adds DAG workflow orchestration, approval gates, and full observability — all without changing your database.",
    differentiators: [
      {
        title: "Queue backend",
        strait: "Postgres with Postgres-backed — no Redis to provision or maintain",
        competitor:
          "Requires a dedicated Redis instance with persistence configured",
      },
      {
        title: "Workflow orchestration",
        strait:
          "Full DAG support with step dependencies, conditions, and fan-out/fan-in",
        competitor:
          "Linear job chains and basic parent-child relationships only",
      },
      {
        title: "Approval gates",
        strait:
          "Built-in human-in-the-loop approvals with timeout and escalation",
        competitor: "No native approval workflow support",
      },
    ],
    categories: [
      {
        name: "Architecture",
        features: [
          {
            feature: "No Redis dependency",
            strait: true,
            competitor: false,
          },
          {
            feature: "Transactional job enqueue",
            strait: true,
            competitor: false,
          },
          {
            feature: "Full run lifecycle tracking",
            strait: true,
            competitor: "Limited states",
          },
          {
            feature: "Dead letter queue",
            strait: true,
            competitor: true,
          },
        ],
      },
      {
        name: "Orchestration Features",
        features: [
          {
            feature: "DAG workflow support",
            strait: true,
            competitor: false,
          },
          {
            feature: "Step conditions and branching",
            strait: true,
            competitor: false,
          },
          {
            feature: "Human approval gates",
            strait: true,
            competitor: false,
          },
          {
            feature: "Cost budget enforcement",
            strait: true,
            competitor: false,
          },
          {
            feature: "Retry with exponential backoff and jitter",
            strait: true,
            competitor: true,
          },
        ],
      },
    ],
    switchingSteps: [
      "Replace BullMQ job definitions with Strait task and workflow configurations using the Strait SDK.",
      "Remove your Redis dependency and connect Strait to your existing Postgres database.",
      "Convert job chains to Strait DAGs with step conditions, approval gates, and retry policies.",
    ],
  },
  {
    slug: "celery",
    competitor: "Celery",
    tagline: "Modern, type-safe, built-in observability",
    description:
      "Celery has served the Python ecosystem well, but its broker-dependent architecture and limited observability show their age. Strait offers a modern runtime with built-in dashboards, structured logging, and full lifecycle tracking — no Flower or external monitoring needed.",
    differentiators: [
      {
        title: "Observability",
        strait:
          "Built-in dashboard with run tracing, debug bundles, and step-level telemetry",
        competitor:
          "Requires Flower or third-party monitoring for basic visibility",
      },
      {
        title: "Type safety",
        strait:
          "TypeScript, Go & Python SDKs with type checking and structured task definitions",
        competitor:
          "Dynamic Python with runtime errors and no task schema validation",
      },
      {
        title: "Broker simplicity",
        strait: "Postgres-only — no RabbitMQ or Redis broker to manage",
        competitor:
          "Requires RabbitMQ, Redis, or SQS as a separate message broker",
      },
    ],
    categories: [
      {
        name: "Runtime & Observability",
        features: [
          {
            feature: "Built-in observability dashboard",
            strait: true,
            competitor: false,
          },
          {
            feature: "Debug bundles for failed runs",
            strait: true,
            competitor: false,
          },
          {
            feature: "Full run lifecycle tracking",
            strait: true,
            competitor: "Limited states",
          },
          {
            feature: "Structured logging",
            strait: true,
            competitor: false,
          },
        ],
      },
      {
        name: "Architecture & Workflows",
        features: [
          {
            feature: "No external broker required",
            strait: true,
            competitor: false,
          },
          {
            feature: "DAG workflow orchestration",
            strait: true,
            competitor: "Canvas primitives only",
          },
          {
            feature: "Human-in-the-loop approvals",
            strait: true,
            competitor: false,
          },
          {
            feature: "Cost budget enforcement",
            strait: true,
            competitor: false,
          },
          {
            feature: "Retry strategies with jitter",
            strait: true,
            competitor: true,
          },
        ],
      },
    ],
    switchingSteps: [
      "Map Celery tasks and Canvas chains to Strait workflow DAGs with step conditions.",
      "Remove your RabbitMQ or Redis broker and point Strait at your Postgres database.",
      "Replace Flower with Strait's built-in observability dashboard for run tracing and debugging.",
      "Deploy workers and validate end-to-end with the CLI and TUI tools.",
    ],
  },
  {
    slug: "sidekiq",
    competitor: "Sidekiq",
    tagline: "Multi-language, no Redis, workflow orchestration",
    description:
      "Sidekiq is the go-to for Ruby background jobs, but it ties you to Redis and the Ruby runtime. Strait brings workflow orchestration to the Go ecosystem with Postgres-backed queues, DAG support, and approval gates — no Redis or Ruby required.",
    differentiators: [
      {
        title: "Runtime ecosystem",
        strait:
          "TypeScript, Go & Python SDKs with lightweight workers and compile-time safety",
        competitor: "Ruby-only with GIL limitations and runtime type errors",
      },
      {
        title: "Queue backend",
        strait:
          "Postgres with Postgres-backed — no Redis memory limits or persistence concerns",
        competitor:
          "Redis-dependent with memory-based queuing and AOF/RDB persistence trade-offs",
      },
      {
        title: "Workflow support",
        strait:
          "Full DAG orchestration with conditions, approvals, and fan-out/fan-in",
        competitor: "Sidekiq Pro/Enterprise required for basic batches",
      },
    ],
    categories: [
      {
        name: "Infrastructure",
        features: [
          {
            feature: "No Redis dependency",
            strait: true,
            competitor: false,
          },
          {
            feature: "Runs on Postgres only",
            strait: true,
            competitor: false,
          },
          {
            feature: "Language-agnostic API",
            strait: true,
            competitor: "Ruby only",
          },
          {
            feature: "Self-hosted with no license fees",
            strait: true,
            competitor: "Pro/Enterprise paid",
          },
        ],
      },
      {
        name: "Job & Workflow Features",
        features: [
          {
            feature: "DAG workflow orchestration",
            strait: true,
            competitor: false,
          },
          {
            feature: "Human approval gates",
            strait: true,
            competitor: false,
          },
          {
            feature: "Cost budget enforcement",
            strait: true,
            competitor: false,
          },
          {
            feature: "Dead letter queue and replay",
            strait: true,
            competitor: true,
          },
          {
            feature: "Built-in observability",
            strait: true,
            competitor: "Sidekiq Web UI",
          },
        ],
      },
    ],
    switchingSteps: [
      "Translate Sidekiq worker classes to Strait task definitions using the Strait SDK.",
      "Replace Redis with your existing Postgres database as the queue backend.",
      "Convert Sidekiq Pro batches to Strait DAG workflows with step conditions and approval gates.",
    ],
  },
  {
    slug: "aws-step-functions",
    competitor: "AWS Step Functions",
    tagline: "No cloud lock-in, cost transparency",
    description:
      "AWS Step Functions integrate tightly with the AWS ecosystem but lock you into a single cloud vendor with opaque per-transition pricing. Strait runs anywhere, gives you full cost visibility, and stores everything in Postgres — no cloud vendor dependency.",
    differentiators: [
      {
        title: "Cloud portability",
        strait:
          "Run on any infrastructure — AWS, GCP, Azure, bare metal, or local development",
        competitor:
          "Locked to AWS with deep service coupling and no portability path",
      },
      {
        title: "Cost model",
        strait:
          "Predictable Postgres-based costs with built-in budget enforcement per workflow",
        competitor:
          "Per-state-transition pricing that scales unpredictably with workflow complexity",
      },
      {
        title: "Workflow definition",
        strait:
          "Multi-language SDKs with type-safe DAGs, step conditions, and approval gates",
        competitor:
          "JSON-based Amazon States Language with limited IDE support",
      },
    ],
    categories: [
      {
        name: "Portability & Cost",
        features: [
          {
            feature: "Cloud-agnostic deployment",
            strait: true,
            competitor: false,
          },
          {
            feature: "Predictable pricing model",
            strait: true,
            competitor: false,
          },
          {
            feature: "Per-workflow cost budgets",
            strait: true,
            competitor: false,
          },
          {
            feature: "Self-hosted option",
            strait: true,
            competitor: false,
          },
        ],
      },
      {
        name: "Developer Experience",
        features: [
          {
            feature: "Type-safe workflow definitions",
            strait: true,
            competitor: false,
          },
          {
            feature: "Local development and testing",
            strait: true,
            competitor: "SAM/LocalStack required",
          },
          {
            feature: "CLI and TUI tooling",
            strait: true,
            competitor: "AWS CLI only",
          },
          {
            feature: "Human-in-the-loop approvals",
            strait: true,
            competitor: "Via callback pattern",
          },
          {
            feature: "Debug bundles and execution tracing",
            strait: true,
            competitor: "CloudWatch integration",
          },
        ],
      },
    ],
    switchingSteps: [
      "Convert Amazon States Language definitions to Strait DAG workflows using the Strait SDK.",
      "Deploy Strait on your preferred infrastructure and connect it to Postgres.",
      "Replace AWS service integrations with Strait task steps that call your services directly.",
      "Remove Step Functions resources and reclaim per-transition costs.",
    ],
  },
];

export function getAllComparisonSlugs(): string[] {
  return comparisons.map((c) => c.slug);
}

export function getComparisonBySlug(
  slug: string
): ComparisonPageData | undefined {
  return comparisons.find((c) => c.slug === slug);
}
