export type UseCasePageData = {
  slug: string;
  title: string;
  headline: string;
  problem: string;
  solution: string;
  relevantFeatures: Array<{
    name: string;
    slug: string;
    description: string;
  }>;
  workflowSteps: Array<{
    name: string;
    description: string;
  }>;
};

export const USE_CASE_PAGES: UseCasePageData[] = [
  {
    slug: "ai-agent-workflows",
    title: "AI Agent Workflows",
    headline: "Budget-aware orchestration for AI agents.",
    problem:
      "AI agent runs spiral into uncontrolled costs, lack approval gates for sensitive actions, and lose all context when failures occur mid-execution.",
    solution:
      "Strait gives every agent run a cost budget, durable checkpoints, and human approval gates — so agents execute reliably without surprise bills or silent failures.",
    relevantFeatures: [
      {
        name: "Cost Budgets",
        slug: "cost-budgets",
        description:
          "Track token usage and enforce per-run and daily cost limits in real time.",
      },
      {
        name: "Approval Gates",
        slug: "approval-gates",
        description:
          "Pause workflows for manual approval before executing sensitive actions.",
      },
      {
        name: "SDK & API",
        slug: "sdk-endpoints",
        description:
          "Heartbeats, checkpoints, and structured logs through the TypeScript, Go & Python SDKs.",
      },
      {
        name: "Workflow DAGs",
        slug: "workflow-dags",
        description:
          "Wire multi-step agent plans as directed acyclic graphs with conditions.",
      },
    ],
    workflowSteps: [
      {
        name: "Receive prompt",
        description: "Agent receives the user prompt and parses intent.",
      },
      {
        name: "Plan steps",
        description:
          "Break the prompt into an execution plan with dependencies.",
      },
      {
        name: "Execute with budget",
        description:
          "Run each step while tracking token costs against the budget.",
      },
      {
        name: "Approval gate",
        description:
          "Pause for human review before committing sensitive actions.",
      },
      {
        name: "Deliver result",
        description:
          "Return the final output and report total cost and execution trace.",
      },
    ],
  },
  {
    slug: "background-processing",
    title: "Background Job Processing",
    headline: "One queue for every background job.",
    problem:
      "Background jobs are scattered across multiple queue systems with inconsistent retry behavior and no unified monitoring across services.",
    solution:
      "Strait consolidates all background work into a single PostgreSQL-backed queue with consistent retries, dead-letter routing, and real-time health visibility.",
    relevantFeatures: [
      {
        name: "PostgreSQL Queue",
        slug: "postgresql-queue",
        description:
          "Your existing Postgres becomes a high-throughput job queue — no extra infrastructure.",
      },
      {
        name: "Retries & DLQ",
        slug: "retries-dlq",
        description:
          "Exponential backoff with jitter and automatic dead-letter routing.",
      },
      {
        name: "SDK & API",
        slug: "sdk-endpoints",
        description:
          "Health scoring auto-computed from heartbeats, logs, and telemetry.",
      },
      {
        name: "Real-time CDC",
        slug: "real-time-cdc",
        description:
          "Stream every state change to your webhooks in real time.",
      },
    ],
    workflowSteps: [
      {
        name: "API receives request",
        description: "Your application receives an incoming API request.",
      },
      {
        name: "Enqueue job",
        description:
          "Push the work item onto the PostgreSQL queue with priority.",
      },
      {
        name: "Worker claims",
        description:
          "A worker picks up the job with exactly-once delivery guarantees.",
      },
      {
        name: "Execute",
        description:
          "The worker processes the job, emitting heartbeats and logs.",
      },
      {
        name: "Report result",
        description:
          "Mark the run as completed and stream the result via CDC webhook.",
      },
    ],
  },
  {
    slug: "data-pipelines",
    title: "Data Pipeline Orchestration",
    headline: "Orchestrate every stage of your pipeline.",
    problem:
      "Complex dependency chains between extraction, transformation, and loading stages fail silently, cascade unpredictably, and require manual restarts.",
    solution:
      "Strait models your pipeline as a workflow DAG with automatic retries, real-time state streaming, and one-click replay from any failed stage.",
    relevantFeatures: [
      {
        name: "Workflow DAGs",
        slug: "workflow-dags",
        description:
          "Fan-in, fan-out, conditions, and template variables in a directed acyclic graph.",
      },
      {
        name: "Retries & DLQ",
        slug: "retries-dlq",
        description:
          "Configurable retry strategies with dead-letter routing on exhaustion.",
      },
      {
        name: "Real-time CDC",
        slug: "real-time-cdc",
        description:
          "Stream every stage transition to monitoring systems via webhooks.",
      },
      {
        name: "SDK & API",
        slug: "sdk-endpoints",
        description:
          "Checkpoints and continuation let pipelines resume from the last successful stage.",
      },
    ],
    workflowSteps: [
      {
        name: "Trigger extraction",
        description:
          "Kick off data extraction from source systems on schedule or event.",
      },
      {
        name: "Transform data",
        description:
          "Apply transformations with parallel fan-out for independent streams.",
      },
      {
        name: "Load to warehouse",
        description:
          "Write transformed data to the destination warehouse or lake.",
      },
      {
        name: "Validate",
        description:
          "Run data quality checks and flag anomalies before downstream use.",
      },
      {
        name: "Notify",
        description:
          "Send completion or failure notifications to stakeholders via webhook.",
      },
    ],
  },
  {
    slug: "payment-processing",
    title: "Payment & Order Processing",
    headline: "Payments that retry correctly.",
    problem:
      "Payment failures need careful retry logic, idempotency is difficult to implement correctly, and audit trails end up scattered across services.",
    solution:
      "Strait provides idempotent job execution, configurable retry strategies tuned for payment processors, approval gates for high-value orders, and a complete audit trail via CDC.",
    relevantFeatures: [
      {
        name: "PostgreSQL Queue",
        slug: "postgresql-queue",
        description:
          "Idempotency keys and transactional enqueue prevent duplicate charges.",
      },
      {
        name: "Retries & DLQ",
        slug: "retries-dlq",
        description:
          "Exponential backoff with jitter matched to payment processor rate limits.",
      },
      {
        name: "Approval Gates",
        slug: "approval-gates",
        description:
          "Require manual approval for high-value orders before fulfillment.",
      },
      {
        name: "Real-time CDC",
        slug: "real-time-cdc",
        description:
          "Stream every payment state change for a complete, immutable audit trail.",
      },
    ],
    workflowSteps: [
      {
        name: "Validate order",
        description:
          "Check inventory, pricing, and customer details before charging.",
      },
      {
        name: "Charge payment",
        description:
          "Submit the charge with idempotency key and retry on transient failures.",
      },
      {
        name: "Approval if >$500",
        description:
          "Route high-value orders through a human approval gate before fulfillment.",
      },
      {
        name: "Fulfill",
        description:
          "Trigger fulfillment workflows once payment and approval clear.",
      },
      {
        name: "Confirm",
        description:
          "Send order confirmation and stream the event to downstream systems.",
      },
    ],
  },
  {
    slug: "scheduled-tasks",
    title: "Scheduled Task Management",
    headline: "Cron jobs with full visibility.",
    problem:
      "Cron jobs are spread across services with no centralized visibility into execution status and failures go unnoticed until customers report them.",
    solution:
      "Strait replaces scattered cron with a single scheduler backed by PostgreSQL, automatic health scoring, retry logic, and instant alerts on failure.",
    relevantFeatures: [
      {
        name: "PostgreSQL Queue",
        slug: "postgresql-queue",
        description:
          "Cron expressions and one-off scheduling built into the queue layer.",
      },
      {
        name: "SDK & API",
        slug: "sdk-endpoints",
        description:
          "Health scoring auto-computed from heartbeats and execution telemetry.",
      },
      {
        name: "Retries & DLQ",
        slug: "retries-dlq",
        description:
          "Automatic retries with dead-letter routing when scheduled tasks fail.",
      },
      {
        name: "Real-time CDC",
        slug: "real-time-cdc",
        description:
          "Instant webhook alerts on task failure or missed schedule windows.",
      },
    ],
    workflowSteps: [
      {
        name: "Schedule fires",
        description:
          "The cron scheduler triggers the job at the configured interval.",
      },
      {
        name: "Claim job",
        description:
          "A worker claims the run with exactly-once execution guarantees.",
      },
      {
        name: "Execute",
        description:
          "The task runs with heartbeats reporting progress back to Strait.",
      },
      {
        name: "Report health",
        description:
          "Health score updates automatically based on execution telemetry.",
      },
      {
        name: "Alert on failure",
        description:
          "Webhook fires immediately if the task fails or misses its window.",
      },
    ],
  },
];

export function getAllUseCaseSlugs(): string[] {
  return USE_CASE_PAGES.map((u) => u.slug);
}

export function getUseCaseBySlug(slug: string): UseCasePageData | undefined {
  return USE_CASE_PAGES.find((u) => u.slug === slug);
}
