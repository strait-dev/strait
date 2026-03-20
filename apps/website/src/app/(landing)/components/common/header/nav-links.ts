export type NavLink = {
  label: string;
  href: string;
  description?: string;
};

export type NavGroup = {
  label: string;
  children: {
    groupLabel: string;
    links: NavLink[];
  }[];
  featured?: NavLink;
};

export type NavItem = NavLink | NavGroup;

export function isNavGroup(item: NavItem): item is NavGroup {
  return "children" in item;
}

export const NAV_ITEMS: NavItem[] = [
  {
    label: "Features",
    children: [
      {
        groupLabel: "Core",
        links: [
          {
            label: "PostgreSQL Queue",
            href: "/features/postgresql-queue",
            description: "Postgres-backed job queue",
          },
          {
            label: "Workflow DAGs",
            href: "/features/workflow-dags",
            description: "Dependency orchestration",
          },
          {
            label: "Approval Gates",
            href: "/features/approval-gates",
            description: "Human-in-the-loop",
          },
        ],
      },
      {
        groupLabel: "Reliability",
        links: [
          {
            label: "Retries & DLQ",
            href: "/features/retries-dlq",
            description: "Failure recovery",
          },
          {
            label: "Cost Budgets",
            href: "/features/cost-budgets",
            description: "Spend limits per run",
          },
        ],
      },
      {
        groupLabel: "Integrations",
        links: [
          {
            label: "Real-Time CDC",
            href: "/features/real-time-cdc",
            description: "Stream state changes",
          },
          {
            label: "SDK Endpoints",
            href: "/features/sdk-endpoints",
            description: "Go SDK & API",
          },
        ],
      },
    ],
    featured: { label: "See all features", href: "/features" },
  },
  {
    label: "Solutions",
    children: [
      {
        groupLabel: "Use Cases",
        links: [
          { label: "AI Agents", href: "/use-cases/ai-agent-workflows" },
          {
            label: "Background Jobs",
            href: "/use-cases/background-processing",
          },
          { label: "Data Pipelines", href: "/use-cases/data-pipelines" },
          { label: "Payments", href: "/use-cases/payment-processing" },
          { label: "Scheduled Tasks", href: "/use-cases/scheduled-tasks" },
        ],
      },
      {
        groupLabel: "Compare",
        links: [
          { label: "vs Temporal", href: "/compare/temporal" },
          { label: "vs Inngest", href: "/compare/inngest" },
          { label: "vs Trigger.dev", href: "/compare/trigger-dev" },
          { label: "vs Hatchet", href: "/compare/hatchet" },
          { label: "vs BullMQ", href: "/compare/bullmq" },
          { label: "vs Celery", href: "/compare/celery" },
          { label: "vs Sidekiq", href: "/compare/sidekiq" },
          {
            label: "vs AWS Step Functions",
            href: "/compare/aws-step-functions",
          },
        ],
      },
    ],
  },
  { label: "Pricing", href: "/pricing" },
  { label: "Blog", href: "/blog" },
];

// Legacy compat - flat list for simple use
export const NAV_LINKS = NAV_ITEMS.filter(
  (item): item is NavLink => !isNavGroup(item)
);
