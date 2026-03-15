export type FeaturePageData = {
  slug: string;
  name: string;
  headline: string;
  subheadline: string;
  description: string;
  specs: Array<{ label: string; value: string }>;
  codeExample: {
    title: string;
    language: string;
    code: string;
  };
  relatedSlugs: string[];
};

export const FEATURE_PAGES: FeaturePageData[] = [
  {
    slug: "postgresql-queue",
    name: "PostgreSQL Queue",
    headline: "Queue without a broker.",
    subheadline:
      "SKIP LOCKED turns your existing Postgres into a high-throughput, exactly-once job queue.",
    description:
      "Strait uses PostgreSQL advisory locks and SKIP LOCKED to provide a durable, transactional job queue. No Redis, no RabbitMQ, no SQS — just the database you already run.",
    specs: [
      { label: "Queue mechanism", value: "SELECT ... FOR UPDATE SKIP LOCKED" },
      { label: "Scheduling", value: "Cron expressions + one-off" },
      { label: "Run states", value: "13-state FSM" },
      { label: "Deduplication", value: "Idempotency keys" },
      { label: "Concurrency", value: "Configurable per-job limits" },
      { label: "Priority", value: "Numeric priority queues" },
    ],
    codeExample: {
      title: "Create a job and trigger a run",
      language: "go",
      code: `client.Jobs.Create(ctx, strait.JobInput{
  Name:        "send-invoice",
  Queue:       "billing",
  MaxRetries:  3,
  Backoff:     strait.ExponentialJitter,
  Timeout:     30 * time.Second,
})

client.Runs.Create(ctx, strait.RunInput{
  JobID:   "send-invoice",
  Payload: invoiceJSON,
})`,
    },
    relatedSlugs: ["workflow-dags", "retries-dlq", "real-time-cdc"],
  },
  {
    slug: "workflow-dags",
    name: "Workflow DAGs",
    headline: "Wire any dependency graph.",
    subheadline:
      "Fan-in, fan-out, conditions, and template variables in a directed acyclic graph.",
    description:
      "Define multi-step workflows where steps run in parallel, wait for dependencies, and pass outputs downstream. Strait resolves the execution order automatically.",
    specs: [
      { label: "Step types", value: "Job, sub-workflow, approval" },
      { label: "Parallelism", value: "Fan-out / fan-in" },
      { label: "Conditions", value: "Expression-based step guards" },
      { label: "Variables", value: "Template interpolation" },
      { label: "Output transforms", value: "JQ-style transforms" },
      { label: "Max depth", value: "Unlimited nesting" },
    ],
    codeExample: {
      title: "Define a workflow with dependencies",
      language: "go",
      code: `client.Workflows.Create(ctx, strait.WorkflowInput{
  Name: "order-processing",
  Steps: []strait.Step{
    {ID: "validate", JobID: "validate-payload"},
    {ID: "charge", JobID: "charge-payment", DependsOn: []string{"validate"}},
    {ID: "fulfill", JobID: "fulfill-order", DependsOn: []string{"charge"}},
    {ID: "notify", JobID: "send-email", DependsOn: []string{"fulfill"}},
  },
})`,
    },
    relatedSlugs: ["postgresql-queue", "approval-gates", "sdk-endpoints"],
  },
  {
    slug: "approval-gates",
    name: "Approval Gates",
    headline: "Human-in-the-loop, built in.",
    subheadline:
      "Pause workflows for manual approval, then resume automatically when authorized.",
    description:
      "Add durable approval gates to any workflow step. Runs pause and wait — for minutes or days — until a human approves, rejects, or the timeout fires.",
    specs: [
      { label: "Trigger", value: "API, dashboard, or webhook" },
      { label: "Timeout", value: "Configurable per gate" },
      { label: "Escalation", value: "Auto-reject on timeout" },
      { label: "Audit trail", value: "Full event history" },
      { label: "Notifications", value: "Webhook on state change" },
      { label: "Concurrency", value: "Multiple gates per workflow" },
    ],
    codeExample: {
      title: "Add an approval gate to a workflow step",
      language: "go",
      code: `strait.Step{
  ID:        "review",
  Type:      strait.StepTypeApproval,
  DependsOn: []string{"charge-payment"},
  Approval: &strait.ApprovalConfig{
    Timeout:    24 * time.Hour,
    OnTimeout:  strait.ApprovalReject,
    WebhookURL: "https://api.example.com/approvals",
  },
}`,
    },
    relatedSlugs: ["workflow-dags", "cost-budgets", "sdk-endpoints"],
  },
  {
    slug: "retries-dlq",
    name: "Retries & DLQ",
    headline: "Failures are a feature.",
    subheadline:
      "Exponential backoff with jitter, configurable max attempts, and automatic dead-letter routing.",
    description:
      "Every run gets retried according to its strategy. When retries exhaust, runs route to the dead-letter queue for inspection and replay.",
    specs: [
      { label: "Strategies", value: "Fixed, exponential, exponential+jitter" },
      { label: "Max retries", value: "Configurable per job" },
      { label: "Backoff cap", value: "Configurable maximum delay" },
      { label: "DLQ", value: "Automatic routing on exhaustion" },
      { label: "Replay", value: "One-click from DLQ" },
      { label: "Visibility", value: "Per-attempt event logs" },
    ],
    codeExample: {
      title: "Configure retry strategy",
      language: "go",
      code: `client.Jobs.Create(ctx, strait.JobInput{
  Name:       "charge-payment",
  MaxRetries: 5,
  Backoff:    strait.ExponentialJitter,
  BackoffCap: 30 * time.Second,
  OnExhaust:  strait.MoveToDeadLetter,
})

// Replay from DLQ
client.Runs.Replay(ctx, "run_abc123")`,
    },
    relatedSlugs: ["postgresql-queue", "sdk-endpoints", "real-time-cdc"],
  },
  {
    slug: "cost-budgets",
    name: "Cost Budgets",
    headline: "Spend limits per run.",
    subheadline:
      "Track AI model token usage and enforce per-run and daily cost limits in real time.",
    description:
      "Strait tracks cost usage reported by workers and enforces budget limits. When a run exceeds its budget, it pauses for approval or terminates automatically.",
    specs: [
      { label: "Granularity", value: "Per-run and daily limits" },
      { label: "Tracking", value: "Real-time cost accumulation" },
      { label: "Enforcement", value: "Pause or terminate on breach" },
      { label: "Reporting", value: "Cost per run, job, and workflow" },
      { label: "Models", value: "Any model with token pricing" },
      { label: "Alerts", value: "Webhook on budget threshold" },
    ],
    codeExample: {
      title: "Set a cost budget on a run",
      language: "go",
      code: `client.Runs.Create(ctx, strait.RunInput{
  JobID: "ai-summarize",
  Budget: &strait.Budget{
    MaxCostPerRun: 1200, // $12.00 in cents
    DailyLimit:    5000, // $50.00 in cents
    OnExceed:      strait.BudgetPause,
  },
})

// Report usage from worker
sdk.ReportUsage(ctx, strait.Usage{
  Tokens:    4200,
  CostCents: 84,
  Model:     "gpt-4o",
})`,
    },
    relatedSlugs: ["approval-gates", "sdk-endpoints", "workflow-dags"],
  },
  {
    slug: "real-time-cdc",
    name: "Real-time CDC",
    headline: "Stream every state change.",
    subheadline:
      "Postgres WAL via Sequin streams run state changes to webhooks with HMAC-SHA256 signatures.",
    description:
      "Change Data Capture captures every run state transition from the Postgres WAL and delivers it to your webhooks. No polling, no missed events.",
    specs: [
      { label: "Source", value: "Postgres WAL (logical replication)" },
      { label: "Delivery", value: "Webhooks with retry" },
      { label: "Security", value: "HMAC-SHA256 signatures" },
      { label: "Latency", value: "Sub-second" },
      { label: "Filtering", value: "Per-job and per-status" },
      { label: "Provider", value: "Sequin integration" },
    ],
    codeExample: {
      title: "Register a webhook for CDC events",
      language: "go",
      code: `client.Webhooks.Create(ctx, strait.WebhookInput{
  URL:       "https://api.example.com/events",
  Secret:    "whsec_...",
  Events:    []string{"run.completed", "run.failed"},
  Filter: &strait.WebhookFilter{
    JobIDs: []string{"charge-payment"},
  },
})`,
    },
    relatedSlugs: ["postgresql-queue", "sdk-endpoints", "retries-dlq"],
  },
  {
    slug: "sdk-endpoints",
    name: "SDK & API",
    headline: "First-class Go SDK.",
    subheadline:
      "Logging, heartbeats, checkpoints, continuation, and health scoring through a clean Go client.",
    description:
      "The Strait SDK gives workers a rich telemetry API. Report progress, save checkpoints for resumption, and emit structured logs — all queryable in the dashboard.",
    specs: [
      { label: "Heartbeats", value: "Configurable interval" },
      { label: "Checkpoints", value: "Durable state snapshots" },
      { label: "Logging", value: "Structured key-value logs" },
      { label: "Continuation", value: "Resume from last checkpoint" },
      { label: "Health scoring", value: "Auto-computed from telemetry" },
      { label: "Client", value: "Go SDK + REST API" },
    ],
    codeExample: {
      title: "Worker SDK usage",
      language: "go",
      code: `func handler(ctx context.Context, run strait.Run) error {
  sdk := run.SDK()
  sdk.Log(ctx, "starting", strait.KV{"order_id", run.Payload["id"]})
  sdk.Heartbeat(ctx)

  result, err := processOrder(ctx, run.Payload)
  if err != nil {
    sdk.Checkpoint(ctx, strait.Checkpoint{
      Step:  "process",
      State: partialState,
    })
    return err
  }

  sdk.Log(ctx, "completed", strait.KV{"total", result.Total})
  return nil
}`,
    },
    relatedSlugs: ["postgresql-queue", "workflow-dags", "cost-budgets"],
  },
];

export function getFeatureBySlug(slug: string): FeaturePageData | undefined {
  return FEATURE_PAGES.find((f) => f.slug === slug);
}

export function getAllFeatureSlugs(): string[] {
  return FEATURE_PAGES.map((f) => f.slug);
}
