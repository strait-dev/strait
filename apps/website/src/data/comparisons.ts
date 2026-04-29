export type ComparisonCategory = {
  name: string;
  features: Array<{
    feature: string;
    strait: string | boolean;
    competitor: string | boolean;
    tooltip?: string;
  }>;
};

export type ComparisonHighlight = {
  title: string;
  description: string;
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
  highlights: ComparisonHighlight[];
  switchingSteps: string[];
};

const comparisons: ComparisonPageData[] = [
  {
    slug: "temporal",
    competitor: "Temporal",
    tagline: "Simpler ops -- just Postgres, no cluster",
    description:
      "Temporal is a powerful durable execution platform with 7 language SDKs and flexible persistence backends. Strait delivers similar durability guarantees with just Postgres -- no cluster to provision, scale, or monitor.",
    differentiators: [
      {
        title: "Infrastructure footprint",
        strait: "Single Postgres instance with Postgres-backed queuing",
        competitor:
          "Multi-node cluster with pluggable persistence (PostgreSQL, MySQL, Cassandra, SQLite)",
      },
      {
        title: "Operational complexity",
        strait:
          "Zero-config queue backed by your existing database -- no cluster management",
        competitor:
          "Cluster requires operational expertise for upgrades, schema migrations, and scaling",
      },
      {
        title: "Workflow modeling",
        strait:
          "Declarative DAGs with step conditions, approval gates, and cost budgets",
        competitor:
          "Imperative code-based workflows using language-specific SDKs with child workflow composition",
      },
    ],
    categories: [
      {
        name: "Infrastructure",
        features: [
          {
            feature: "Runs on Postgres only",
            strait: true,
            competitor: false,
            tooltip:
              "Temporal supports PostgreSQL, MySQL, Cassandra, and SQLite as persistence backends",
          },
          {
            feature: "No external cluster required",
            strait: true,
            competitor: false,
          },
          {
            feature: "Sub-minute setup",
            strait: true,
            competitor: false,
          },
          {
            feature: "Self-hosting",
            strait: "Simple (Docker)",
            competitor: "Supported (cluster ops)",
          },
        ],
      },
      {
        name: "Workflow Capabilities",
        features: [
          {
            feature: "DAG workflow orchestration",
            strait: "Declarative DAGs",
            competitor: "Via child workflows and composition",
          },
          {
            feature: "Human-in-the-loop",
            strait: "Built-in approval gates",
            competitor: "Via Signals and Activities",
          },
          {
            feature: "Cost budget enforcement",
            strait: true,
            competitor: false,
          },
          {
            feature: "Retry strategies",
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
      {
        name: "Developer Experience",
        features: [
          {
            feature: "Language SDKs",
            strait: "5 (TypeScript, Python, Go, Ruby, Rust)",
            competitor: "7 (Go, Java, PHP, Python, TypeScript, .NET, Ruby)",
          },
          {
            feature: "CLI and TUI tooling",
            strait: true,
            competitor: "CLI tools (tctl)",
          },
          {
            feature: "Workflow definition style",
            strait: "Declarative (JSON/SDK)",
            competitor: "Imperative (code)",
          },
        ],
      },
      {
        name: "Observability",
        features: [
          {
            feature: "Built-in dashboard",
            strait: true,
            competitor: "Web UI included",
          },
          {
            feature: "Debug bundles",
            strait: true,
            competitor: false,
          },
          {
            feature: "Step-level telemetry",
            strait: true,
            competitor: true,
          },
        ],
      },
    ],
    highlights: [
      {
        title: "Declarative DAGs",
        description:
          "Define workflows as declarative DAGs with step conditions and branching, instead of writing imperative code for every workflow path.",
      },
      {
        title: "First-class approval gates",
        description:
          "Built-in human-in-the-loop approval steps with timeout and escalation -- no custom Signal handlers or Activity implementations needed.",
      },
      {
        title: "Single Postgres simplicity",
        description:
          "No cluster to provision or manage. Your existing Postgres database is the only infrastructure dependency.",
      },
      {
        title: "Cost budgets",
        description:
          "Set per-workflow cost budgets to prevent runaway spending on AI and compute resources.",
      },
    ],
    switchingSteps: [
      "Export your Temporal workflow definitions and map them to Strait DAG steps with conditions and approval gates.",
      "Point Strait at your existing Postgres database -- no Cassandra, Elasticsearch, or cluster provisioning needed.",
      "Deploy workers using the Strait SDK and validate runs through the built-in observability dashboard.",
      "Decommission your Temporal cluster and reclaim infrastructure spend.",
    ],
  },
  {
    slug: "inngest",
    competitor: "Inngest",
    tagline: "Apache 2.0, own your data, multi-language SDKs",
    description:
      "Inngest offers a polished developer experience with 4 language SDKs and self-hosting support. Strait differentiates with a permissive Apache 2.0 license (vs SSPL), Postgres-only architecture, and built-in CLI/TUI tooling.",
    differentiators: [
      {
        title: "License",
        strait: "Apache 2.0 -- fully permissive open source",
        competitor:
          "SSPL for server (restrictive for SaaS), Apache 2.0 for SDKs only",
      },
      {
        title: "Data ownership",
        strait:
          "All data lives in your Postgres -- full control, no external dependencies",
        competitor:
          "Self-hosting available, but cloud is the primary deployment model",
      },
      {
        title: "Developer tooling",
        strait:
          "Built-in CLI and TUI for local development, debugging, and deployment",
        competitor: "Dashboard-focused workflow management",
      },
    ],
    categories: [
      {
        name: "Hosting & Control",
        features: [
          {
            feature: "Self-hosting",
            strait: "Simple (Docker)",
            competitor: "Supported (documented)",
            tooltip:
              "Inngest server is self-hostable with documented setup guides",
          },
          {
            feature: "License",
            strait: "Apache 2.0",
            competitor: "SSPL (server) / Apache 2.0 (SDKs)",
            tooltip:
              "SSPL restricts offering the software as a managed service",
          },
          {
            feature: "Data stays in your database",
            strait: true,
            competitor: "Depends on deployment",
          },
          {
            feature: "No vendor lock-in",
            strait: true,
            competitor: "Partial (SSPL limits)",
          },
        ],
      },
      {
        name: "Developer Experience",
        features: [
          {
            feature: "Language SDKs",
            strait: "5 (TypeScript, Python, Go, Ruby, Rust)",
            competitor: "4 (TypeScript, Python, Go, Kotlin/Java)",
          },
          {
            feature: "Built-in CLI and TUI",
            strait: true,
            competitor: false,
          },
          {
            feature: "Declarative DAG workflows",
            strait: true,
            competitor: true,
          },
          {
            feature: "Step-level observability",
            strait: true,
            competitor: true,
          },
        ],
      },
      {
        name: "Workflow Features",
        features: [
          {
            feature: "Human-in-the-loop",
            strait: "Built-in approval gates",
            competitor: "Via step.waitForEvent()",
          },
          {
            feature: "Cost budget controls",
            strait: true,
            competitor: false,
          },
          {
            feature: "Retry strategies",
            strait: true,
            competitor: true,
          },
        ],
      },
      {
        name: "AI & Cost",
        features: [
          {
            feature: "AI agent workflows",
            strait: "DAG-based orchestration",
            competitor: "AgentKit",
            tooltip:
              "Inngest provides AgentKit specifically designed for AI agent workflow orchestration",
          },
          {
            feature: "Per-workflow cost budgets",
            strait: true,
            competitor: false,
          },
          {
            feature: "Postgres-only backend",
            strait: true,
            competitor: false,
          },
        ],
      },
    ],
    highlights: [
      {
        title: "Apache 2.0 license",
        description:
          "Fully permissive open-source license with no restrictions on how you deploy or offer the software, unlike SSPL.",
      },
      {
        title: "CLI and TUI tooling",
        description:
          "Built-in command-line and terminal UI tools for local development, debugging, and deployment workflows.",
      },
      {
        title: "Postgres-only architecture",
        description:
          "No external dependencies beyond Postgres. Your data stays in one place with full transactional guarantees.",
      },
      {
        title: "Cost budgets",
        description:
          "Set per-workflow cost budgets to prevent runaway spending on AI and compute resources.",
      },
    ],
    switchingSteps: [
      "Map your Inngest functions to Strait workflow DAGs with step conditions and retry policies.",
      "Deploy Strait on your infrastructure and connect it to your Postgres database.",
      "Migrate event triggers to Strait's API-driven or schedule-based invocation model.",
    ],
  },
  {
    slug: "trigger-dev",
    competitor: "Trigger.dev",
    tagline: "Multi-language SDKs, DAG orchestration, approval gates",
    description:
      "Trigger.dev is a TypeScript-first serverless background job platform with self-hosting support and a React Realtime API. Strait adds multi-language SDK support, full DAG orchestration, and a Postgres-only backend.",
    differentiators: [
      {
        title: "Language support",
        strait:
          "5 SDKs: TypeScript, Python, Go, Ruby, Rust -- build in the language your team knows",
        competitor:
          "TypeScript only -- teams using other languages need a different solution",
      },
      {
        title: "Workflow orchestration",
        strait:
          "Full DAG support with step dependencies, conditions, fan-out/fan-in, and approval gates",
        competitor:
          "Imperative code chaining with batchTriggerAndWait() for fan-out/fan-in and subtask orchestration",
      },
      {
        title: "Backend architecture",
        strait:
          "Postgres-only -- no external dependencies beyond your database",
        competitor:
          "Self-hosting requires Postgres + Redis + object storage (Docker Compose or Helm/K8s)",
      },
    ],
    categories: [
      {
        name: "Language Support",
        features: [
          {
            feature: "Language SDKs",
            strait: "5 (TypeScript, Python, Go, Ruby, Rust)",
            competitor: "1 (TypeScript)",
          },
          {
            feature: "Type-safe task definitions",
            strait: true,
            competitor: true,
          },
        ],
      },
      {
        name: "Workflow Orchestration",
        features: [
          {
            feature: "DAG workflow support",
            strait: "Declarative DAGs",
            competitor: "Imperative (code chaining)",
            tooltip:
              "Trigger.dev uses imperative code with task dependencies and subtask orchestration rather than declarative DAG definitions",
          },
          {
            feature: "Step conditions and branching",
            strait: "Declarative conditions",
            competitor: "Implicit (JavaScript control flow)",
            tooltip:
              "Trigger.dev uses standard JavaScript if/else for branching within tasks",
          },
          {
            feature: "Human-in-the-loop",
            strait: "Built-in approval gates",
            competitor: "Via wait.forToken()",
            tooltip:
              "Trigger.dev supports pausing tasks until an external token is received",
          },
          {
            feature: "Fan-out / fan-in",
            strait: true,
            competitor: "Via batchTriggerAndWait()",
            tooltip:
              "Trigger.dev supports fan-out/fan-in patterns through batchTriggerAndWait() for parallel task execution",
          },
          {
            feature: "Cost budget enforcement",
            strait: true,
            competitor: false,
          },
        ],
      },
      {
        name: "Infrastructure",
        features: [
          {
            feature: "Postgres-only backend",
            strait: true,
            competitor: false,
            tooltip:
              "Trigger.dev self-hosting requires Postgres + Redis + object storage",
          },
          {
            feature: "Self-hosting",
            strait: "Simple (Docker)",
            competitor: "Docker Compose or Helm/K8s",
          },
          {
            feature: "Execution model",
            strait: "Containers with warm pools",
            competitor: "Serverless (checkpoint-resume)",
            tooltip:
              "Trigger.dev uses CRIU-based checkpoint-resume for durable execution with no execution timeouts",
          },
        ],
      },
      {
        name: "AI & Compute",
        features: [
          {
            feature: "AI agent workflows",
            strait: "DAG-based orchestration",
            competitor: "Built-in (tool calling, streaming)",
            tooltip:
              "Trigger.dev has significant AI agent support including tool calling, prompt chaining, streaming, and framework integrations",
          },
          {
            feature: "Concurrency controls",
            strait: true,
            competitor: true,
            tooltip:
              "Trigger.dev provides custom queues, concurrency limits, and batch triggering",
          },
          {
            feature: "Cost budget enforcement",
            strait: true,
            competitor: false,
          },
          {
            feature: "Durable execution",
            strait: true,
            competitor: true,
            tooltip:
              "Trigger.dev provides durable execution via CRIU checkpoint-resume with no execution timeouts",
          },
        ],
      },
      {
        name: "Observability",
        features: [
          {
            feature: "Built-in dashboard",
            strait: true,
            competitor: true,
          },
          {
            feature: "Realtime API",
            strait: false,
            competitor: "React hooks",
            tooltip:
              "Trigger.dev provides a Realtime API with React hooks for live job status updates",
          },
          {
            feature: "Debug bundles",
            strait: true,
            competitor: false,
          },
          {
            feature: "CLI and TUI tooling",
            strait: true,
            competitor: "CLI available",
          },
        ],
      },
    ],
    highlights: [
      {
        title: "5 language SDKs",
        description:
          "Build workflows in TypeScript, Python, Go, Ruby, or Rust -- not just TypeScript.",
      },
      {
        title: "DAG orchestration",
        description:
          "Full directed acyclic graph workflows with step dependencies, conditions, and fan-out/fan-in patterns.",
      },
      {
        title: "Postgres-only backend",
        description:
          "All data and queuing backed by Postgres. No additional infrastructure to manage.",
      },
      {
        title: "Built-in approval gates",
        description:
          "First-class human-in-the-loop approval steps with timeout and escalation, without external token coordination.",
      },
    ],
    switchingSteps: [
      "Map your Trigger.dev tasks to Strait workflow DAGs with step conditions and retry policies.",
      "Deploy Strait on your infrastructure and connect it to your Postgres database.",
      "Migrate triggers and schedules to Strait's API-driven or schedule-based invocation model.",
      "Replace TypeScript-specific patterns with multi-language SDK equivalents where needed.",
    ],
  },
  {
    slug: "hatchet",
    competitor: "Hatchet",
    tagline: "More SDKs, built-in approval gates, cost budgets",
    description:
      "Hatchet is a Postgres-based task orchestrator with DAG support, dynamic rate limiting via CEL expressions, and strong AI agent features. Strait differentiates with more language SDKs (5 vs 3), first-class approval gates, CLI/TUI tooling, and per-workflow cost budgets.",
    differentiators: [
      {
        title: "Language ecosystem",
        strait:
          "5 SDKs: TypeScript, Python, Go, Ruby, Rust -- broader language coverage",
        competitor: "3 SDKs: Go, Python, TypeScript -- no Ruby or Rust support",
      },
      {
        title: "Approval workflows",
        strait:
          "First-class approval gates with dedicated UX, timeout, and escalation",
        competitor:
          "Human-in-the-loop via generic durable events with CEL-based conditions",
      },
      {
        title: "Developer tooling",
        strait:
          "Built-in CLI and TUI for local development, debugging, and deployment",
        competitor: "Dashboard-focused workflow management and monitoring",
      },
    ],
    categories: [
      {
        name: "Infrastructure",
        features: [
          {
            feature: "Primary backend",
            strait: "Postgres-only",
            competitor: "PostgreSQL primary",
            tooltip:
              "Both are Postgres-based. Hatchet optionally uses RabbitMQ for higher throughput",
          },
          {
            feature: "Self-hosting",
            strait: "Simple (Docker)",
            competitor: "Docker Compose, Kubernetes/Helm",
          },
          {
            feature: "License",
            strait: "Apache 2.0",
            competitor: "MIT",
            tooltip:
              "MIT is more permissive than Apache 2.0 -- fewer requirements for attribution and patent grants",
          },
          {
            feature: "No optional message broker",
            strait: true,
            competitor: false,
            tooltip:
              "Hatchet optionally requires RabbitMQ for higher throughput workloads",
          },
        ],
      },
      {
        name: "Workflow Orchestration",
        features: [
          {
            feature: "DAG workflow support",
            strait: true,
            competitor: true,
            tooltip:
              "Both support full DAG workflows with task dependencies and parallel execution",
          },
          {
            feature: "Human-in-the-loop",
            strait: "Built-in approval gates",
            competitor: "Via durable events",
            tooltip:
              "Hatchet supports human-in-the-loop via durable events with CEL-based conditions",
          },
          {
            feature: "Cost budget enforcement",
            strait: true,
            competitor: false,
          },
          {
            feature: "Rate limiting",
            strait: true,
            competitor: "Dynamic (CEL expressions)",
            tooltip:
              "Hatchet supports advanced dynamic rate limiting using CEL expressions, beyond basic static limits",
          },
          {
            feature: "Concurrency strategies",
            strait: true,
            competitor: "GROUP_ROUND_ROBIN, CANCEL_IN_PROGRESS, CANCEL_NEWEST",
            tooltip:
              "Hatchet provides multiple concurrency strategies for fine-grained control",
          },
          {
            feature: "Retry strategies",
            strait: true,
            competitor: true,
          },
        ],
      },
      {
        name: "Developer Experience",
        features: [
          {
            feature: "Language SDKs",
            strait: "5 (TypeScript, Python, Go, Ruby, Rust)",
            competitor: "3 (Go, Python, TypeScript)",
          },
          {
            feature: "CLI and TUI tooling",
            strait: true,
            competitor: false,
          },
          {
            feature: "Type-safe task definitions",
            strait: true,
            competitor: true,
          },
          {
            feature: "Scheduling",
            strait: "Cron and API-driven",
            competitor: "Cron, one-time, event-triggered, durable sleep",
          },
        ],
      },
      {
        name: "Observability & AI",
        features: [
          {
            feature: "Built-in dashboard",
            strait: true,
            competitor: true,
          },
          {
            feature: "Debug bundles",
            strait: true,
            competitor: false,
          },
          {
            feature: "AI agent workflows",
            strait: "DAG-based orchestration",
            competitor: "State checkpointing, tool call orchestration",
            tooltip:
              "Hatchet has strong AI agent positioning with state checkpointing, exactly-once semantics, and tool call orchestration",
          },
          {
            feature: "Alerts and replay",
            strait: true,
            competitor: true,
          },
        ],
      },
    ],
    highlights: [
      {
        title: "5 language SDKs",
        description:
          "Build workflows in TypeScript, Python, Go, Ruby, or Rust -- not limited to 3 languages.",
      },
      {
        title: "First-class approval gates",
        description:
          "Dedicated approval UX with timeout and escalation, instead of generic durable event patterns.",
      },
      {
        title: "CLI and TUI tooling",
        description:
          "Built-in terminal UI for local development, debugging, and deployment workflows.",
      },
      {
        title: "Cost budget enforcement",
        description:
          "Set per-workflow cost budgets to prevent runaway spending on AI and compute resources.",
      },
    ],
    switchingSteps: [
      "Map Hatchet tasks and DAG definitions to Strait workflow steps with conditions and retry policies.",
      "Connect Strait to your existing Postgres database -- no RabbitMQ dependency needed.",
      "Convert durable event patterns to Strait's first-class approval gates with timeout and escalation.",
      "Deploy workers using the Strait SDK and validate runs through the built-in CLI/TUI and dashboard.",
    ],
  },
  {
    slug: "bullmq",
    competitor: "BullMQ",
    tagline: "No Redis dependency, DAG workflows, approvals",
    description:
      "BullMQ is a solid Redis-based queue for Node.js with parent-child job flows and scheduling. Strait replaces Redis with Postgres, adds full DAG workflow orchestration with conditional branching, and provides built-in observability.",
    differentiators: [
      {
        title: "Queue backend",
        strait: "Postgres-backed -- no Redis to provision or maintain",
        competitor:
          "Requires a dedicated Redis instance with persistence configured",
      },
      {
        title: "Workflow orchestration",
        strait:
          "Full DAG support with step dependencies, conditions, and fan-out/fan-in",
        competitor:
          "Parent-child job dependencies via FlowProducer (tree-like, not full DAG)",
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
            feature: "Language support",
            strait: "5 (TypeScript, Python, Go, Ruby, Rust)",
            competitor: "Node.js primary, community Python/PHP/Elixir",
            tooltip:
              "BullMQ has official Node.js support with community implementations for Python, PHP, and Elixir",
          },
          {
            feature: "Full run lifecycle tracking",
            strait: true,
            competitor: "Limited states",
          },
        ],
      },
      {
        name: "Orchestration",
        features: [
          {
            feature: "DAG workflow support",
            strait: "Full DAG with conditions",
            competitor: "Parent-child flows (tree-like)",
            tooltip:
              "BullMQ Flows support parent-child dependencies via FlowProducer, but not conditional branching or fan-out/fan-in",
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
            feature: "Retry with backoff and jitter",
            strait: true,
            competitor: true,
          },
        ],
      },
      {
        name: "Developer Experience",
        features: [
          {
            feature: "Job scheduling (cron)",
            strait: true,
            competitor: true,
          },
          {
            feature: "Job priorities",
            strait: true,
            competitor: true,
          },
          {
            feature: "Dead letter queue",
            strait: true,
            competitor: true,
          },
          {
            feature: "CLI and TUI tooling",
            strait: true,
            competitor: false,
          },
        ],
      },
      {
        name: "Observability",
        features: [
          {
            feature: "Built-in dashboard",
            strait: true,
            competitor: false,
            tooltip:
              "BullMQ has no built-in dashboard; community tools like Bull Board provide basic visibility",
          },
          {
            feature: "Debug bundles",
            strait: true,
            competitor: false,
          },
          {
            feature: "Step-level telemetry",
            strait: true,
            competitor: false,
          },
        ],
      },
    ],
    highlights: [
      {
        title: "No Redis dependency",
        description:
          "All queuing and state management backed by Postgres. No Redis memory limits or persistence trade-offs.",
      },
      {
        title: "Full DAG with conditions",
        description:
          "Go beyond parent-child job trees with conditional branching, fan-out/fan-in, and complex workflow orchestration.",
      },
      {
        title: "Built-in dashboard",
        description:
          "Observability dashboard included out of the box -- no need for community tools like Bull Board.",
      },
      {
        title: "Approval gates",
        description:
          "Built-in human-in-the-loop approval steps with timeout and escalation for workflows that need sign-off.",
      },
    ],
    switchingSteps: [
      "Replace BullMQ job definitions with Strait task and workflow configurations using the Strait SDK.",
      "Remove your Redis dependency and connect Strait to your existing Postgres database.",
      "Convert FlowProducer job trees to Strait DAGs with step conditions, approval gates, and retry policies.",
    ],
  },
  {
    slug: "celery",
    competitor: "Celery",
    tagline: "Modern, type-safe, built-in observability",
    description:
      "Celery has served the Python ecosystem well with Canvas workflow primitives (chord, chain, group, map) and extensive broker/backend support. Strait offers a modern runtime with built-in dashboards, type-safe SDKs, and full lifecycle tracking -- no Flower or external monitoring needed.",
    differentiators: [
      {
        title: "Observability",
        strait:
          "Built-in dashboard with run tracing, debug bundles, and step-level telemetry",
        competitor:
          "Flower (external tool) or third-party monitoring for visibility, plus real-time event stream",
      },
      {
        title: "Type safety",
        strait:
          "TypeScript, Go & Python SDKs with type checking and structured task definitions",
        competitor:
          "Dynamic Python with runtime errors; protocol implementations exist for Node.js, PHP, Go, Rust",
      },
      {
        title: "Broker simplicity",
        strait: "Postgres-only -- no message broker to manage",
        competitor: "Requires RabbitMQ, Redis, SQS, or another message broker",
      },
    ],
    categories: [
      {
        name: "Runtime & Infrastructure",
        features: [
          {
            feature: "No external broker required",
            strait: true,
            competitor: false,
            tooltip:
              "Celery requires a message broker like RabbitMQ, Redis, or SQS",
          },
          {
            feature: "License",
            strait: "Apache 2.0",
            competitor: "BSD 3-Clause",
          },
          {
            feature: "Postgres-only backend",
            strait: true,
            competitor: false,
            tooltip:
              "Celery supports extensive result backends including Redis, SQLAlchemy, Django ORM, Cassandra, Elasticsearch, MongoDB, DynamoDB, and S3",
          },
          {
            feature: "Self-hosting",
            strait: "Simple (Docker)",
            competitor: "You run it (library)",
          },
        ],
      },
      {
        name: "Workflow Composition",
        features: [
          {
            feature: "DAG workflow orchestration",
            strait: "Full DAG with conditions",
            competitor: "Canvas (chord, chain, group, map)",
            tooltip:
              "Celery Canvas provides chord, chain, group, and map primitives for composing workflows",
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
            feature: "Retry strategies",
            strait: true,
            competitor: true,
          },
          {
            feature: "Rate limiting",
            strait: true,
            competitor: true,
          },
          {
            feature: "Scheduled tasks (cron)",
            strait: true,
            competitor: "Via celery-beat",
          },
        ],
      },
      {
        name: "Observability",
        features: [
          {
            feature: "Built-in dashboard",
            strait: true,
            competitor: "Flower (external)",
            tooltip:
              "Flower is a separate tool that must be deployed alongside Celery for monitoring",
          },
          {
            feature: "Debug bundles for failed runs",
            strait: true,
            competitor: false,
          },
          {
            feature: "Full run lifecycle tracking",
            strait: true,
            competitor: "Event stream available",
          },
          {
            feature: "Structured logging",
            strait: true,
            competitor: false,
          },
        ],
      },
      {
        name: "Developer Experience",
        features: [
          {
            feature: "Language SDKs",
            strait: "5 (TypeScript, Python, Go, Ruby, Rust)",
            competitor:
              "Python primary; protocol impls for Node, PHP, Go, Rust",
          },
          {
            feature: "Type-safe task definitions",
            strait: true,
            competitor: false,
          },
          {
            feature: "CLI and TUI tooling",
            strait: true,
            competitor: "celery CLI",
          },
        ],
      },
    ],
    highlights: [
      {
        title: "Built-in dashboard",
        description:
          "Observability included out of the box. No need to deploy and maintain Flower as a separate service.",
      },
      {
        title: "Type-safe SDKs",
        description:
          "Structured task definitions with compile-time type checking across TypeScript, Go, and Python.",
      },
      {
        title: "Postgres-only backend",
        description:
          "No message broker to manage. All queuing and state backed by a single Postgres instance.",
      },
      {
        title: "Approval gates",
        description:
          "Built-in human-in-the-loop approval steps for workflows that need sign-off before continuing.",
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
      "Sidekiq is the go-to for Ruby background jobs with a built-in Web UI, scheduled jobs, and paid tiers for batches and rate limiting. Strait brings workflow orchestration with Postgres-backed queues, DAG support, and approval gates -- no Redis or per-seat licensing required.",
    differentiators: [
      {
        title: "Runtime ecosystem",
        strait:
          "TypeScript, Go & Python SDKs with lightweight workers and compile-time safety",
        competitor: "Ruby-only with built-in Web UI for monitoring",
      },
      {
        title: "Queue backend",
        strait:
          "Postgres-backed -- no Redis memory limits or persistence concerns",
        competitor: "Redis 7.0+ / Valkey / Dragonfly required for all queuing",
      },
      {
        title: "Workflow support",
        strait:
          "Full DAG orchestration with conditions, approvals, and fan-out/fan-in included free",
        competitor:
          "Sidekiq Pro ($99/mo) required for batches; Enterprise ($269/mo+) for rate limiting and cron",
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
            feature: "Language support",
            strait: "5 SDKs (TypeScript, Python, Go, Ruby, Rust)",
            competitor: "Ruby only",
          },
          {
            feature: "License",
            strait: "Apache 2.0",
            competitor: "LGPL (OSS), commercial (Pro/Enterprise)",
          },
        ],
      },
      {
        name: "Job Features",
        features: [
          {
            feature: "Scheduled jobs",
            strait: true,
            competitor: true,
          },
          {
            feature: "Job priorities",
            strait: true,
            competitor: true,
          },
          {
            feature: "Rate limiting",
            strait: true,
            competitor: "Enterprise ($269/mo+)",
            tooltip: "Sidekiq Enterprise ($269/mo+) required for rate limiting",
          },
          {
            feature: "Cron scheduling",
            strait: true,
            competitor: "Enterprise ($269/mo+)",
            tooltip:
              "Sidekiq Enterprise ($269/mo+) required for periodic/cron jobs",
          },
          {
            feature: "Unique jobs",
            strait: true,
            competitor: "Enterprise ($269/mo+)",
            tooltip:
              "Sidekiq Enterprise ($269/mo+) required for unique job deduplication",
          },
        ],
      },
      {
        name: "Workflow & Orchestration",
        features: [
          {
            feature: "DAG workflow orchestration",
            strait: true,
            competitor: false,
          },
          {
            feature: "Batches",
            strait: true,
            competitor: "Pro ($99/mo)",
            tooltip: "Sidekiq Pro ($99/mo) required for batch job processing",
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
        ],
      },
      {
        name: "Governance",
        features: [
          {
            feature: "Built-in observability",
            strait: true,
            competitor: "Web UI included",
          },
          {
            feature: "Dead letter queue and replay",
            strait: true,
            competitor: true,
          },
          {
            feature: "Self-hosted with no license fees",
            strait: true,
            competitor: "Pro/Enterprise paid",
          },
          {
            feature: "Historical metrics",
            strait: true,
            competitor: "Enterprise ($269/mo+)",
          },
        ],
      },
    ],
    highlights: [
      {
        title: "Multi-language SDKs",
        description:
          "Build workflows in TypeScript, Python, Go, Ruby, or Rust -- not locked to Ruby.",
      },
      {
        title: "No Redis dependency",
        description:
          "All queuing backed by Postgres. No Redis memory limits, persistence trade-offs, or Valkey migrations.",
      },
      {
        title: "DAG orchestration included free",
        description:
          "Full workflow orchestration with conditions and fan-out/fan-in included in the open-source core -- no paid tier required.",
      },
      {
        title: "Cost budgets",
        description:
          "Set per-workflow cost budgets to prevent runaway spending on AI and compute resources.",
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
      "AWS Step Functions offer deep AWS integration with 200+ service connectors, visual debugging, and both Standard and Express execution modes. Strait runs anywhere, gives you full cost visibility, and stores everything in Postgres -- no cloud vendor dependency.",
    differentiators: [
      {
        title: "Cloud portability",
        strait:
          "Run on any infrastructure -- AWS, GCP, Azure, bare metal, or local development",
        competitor:
          "Deep AWS integration with 200+ services, but locked to the AWS ecosystem",
      },
      {
        title: "Cost model",
        strait:
          "Predictable Postgres-based costs with built-in budget enforcement per workflow",
        competitor:
          "Per-state-transition pricing (Standard) or per-execution+duration (Express) that scales with complexity",
      },
      {
        title: "Workflow definition",
        strait:
          "Multi-language SDKs with type-safe DAGs, step conditions, and approval gates",
        competitor:
          "JSON-based Amazon States Language with visual editor in the console",
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
            tooltip:
              "Step Functions charge per state transition (Standard) or per execution+duration (Express), which can be hard to predict",
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
        name: "Workflow Definition",
        features: [
          {
            feature: "Type-safe workflow definitions",
            strait: "Multi-language SDKs",
            competitor: "JSON (Amazon States Language)",
          },
          {
            feature: "Parallel execution",
            strait: "Fan-out / fan-in",
            competitor: "Parallel and Map states",
          },
          {
            feature: "Human-in-the-loop",
            strait: "Built-in approval gates",
            competitor: "Via .waitForTaskToken and Activities",
            tooltip:
              "Step Functions support human-in-the-loop via the .waitForTaskToken callback pattern and Activity tasks",
          },
          {
            feature: "Service integrations",
            strait: "API-driven (any service)",
            competitor: "200+ AWS service integrations",
          },
        ],
      },
      {
        name: "Observability",
        features: [
          {
            feature: "Built-in dashboard",
            strait: true,
            competitor: "Visual debugger in AWS Console",
          },
          {
            feature: "Debug bundles",
            strait: true,
            competitor: false,
          },
          {
            feature: "Execution tracing",
            strait: true,
            competitor: "CloudWatch integration",
          },
        ],
      },
      {
        name: "Developer Experience",
        features: [
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
            feature: "Language SDKs",
            strait: "5 (TypeScript, Python, Go, Ruby, Rust)",
            competitor: "AWS SDKs (all major languages)",
          },
        ],
      },
    ],
    highlights: [
      {
        title: "Cloud-agnostic",
        description:
          "Run on any cloud provider, bare metal, or your local machine. No AWS account or vendor lock-in required.",
      },
      {
        title: "Predictable pricing",
        description:
          "Infrastructure costs based on your Postgres usage, not per-state-transition fees that scale unpredictably.",
      },
      {
        title: "Type-safe SDK definitions",
        description:
          "Define workflows with type-safe SDKs instead of JSON-based Amazon States Language.",
      },
      {
        title: "Local development",
        description:
          "Run and test workflows locally without SAM, LocalStack, or any cloud emulation layer.",
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
