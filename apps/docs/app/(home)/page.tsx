import Link from "next/link";

const sections = [
  {
    title: "Getting Started",
    description:
      "Set up Strait in minutes. Learn the architecture and core concepts.",
    href: "/docs/getting-started",
    items: ["Introduction", "Quick Start", "Architecture"],
  },
  {
    title: "Concepts",
    description:
      "Understand jobs, runs, workflows, retry strategies, and event triggers.",
    href: "/docs/concepts",
    items: ["Jobs & Runs", "Workflows & DAGs", "Event Triggers"],
  },
  {
    title: "SDKs",
    description:
      "Official client libraries for TypeScript, Python, Go, Ruby, and Rust.",
    href: "/docs/sdks",
    items: ["TypeScript", "Python", "Go", "Ruby", "Rust"],
  },
  {
    title: "API Reference",
    description:
      "Complete REST API documentation auto-generated from the OpenAPI spec.",
    href: "/docs/api-reference",
    items: ["Jobs", "Runs", "Workflows", "Secrets"],
  },
  {
    title: "CLI",
    description:
      "Command-line interface with 48+ commands for managing jobs, workflows, and deployments.",
    href: "/docs/cli",
    items: ["init", "jobs", "runs", "workflows"],
  },
  {
    title: "Guides",
    description:
      "Step-by-step guides for authentication, deployment, security, and more.",
    href: "/docs/guides",
    items: ["Authentication", "Deployment", "Security"],
  },
];

const features = [
  {
    title: "PostgreSQL-backed Queue",
    description:
      "No external message broker. SELECT FOR UPDATE SKIP LOCKED powers lock-free concurrent workers.",
  },
  {
    title: "Workflow DAGs",
    description:
      "Fan-in/fan-out, step conditions, template variables, approval gates, and durable event waits.",
  },
  {
    title: "Multi-language SDKs",
    description:
      "Full feature parity across TypeScript, Python, Go, Ruby, and Rust.",
  },
  {
    title: "Built for AI Agents",
    description:
      "Cost budgets, checkpoints, continuation, child job spawning, and debug bundles.",
  },
  {
    title: "Single Binary",
    description:
      "One Go executable. No runtime dependencies. Deploy and scale horizontally.",
  },
  {
    title: "Real-time CDC",
    description:
      "Postgres WAL change capture via Sequin. React instantly when jobs or workflows change.",
  },
];

export default function HomePage() {
  return (
    <main className="flex flex-1 flex-col">
      <section className="relative flex flex-col items-center justify-center px-6 py-24 text-center">
        <div className="absolute inset-0 -z-10 bg-gradient-to-b from-primary/5 to-transparent" />
        <p className="mb-4 font-medium text-muted-foreground text-sm uppercase tracking-widest">
          Documentation
        </p>
        <h1 className="max-w-3xl font-bold text-4xl tracking-tight sm:text-5xl lg:text-6xl">
          Build reliable background jobs with{" "}
          <span className="text-primary">Strait</span>
        </h1>
        <p className="mt-6 max-w-2xl text-lg text-muted-foreground">
          A production-grade Go job orchestration platform for engineering teams
          and AI agents. Single binary, PostgreSQL-backed, with workflow DAGs
          and multi-language SDKs.
        </p>
        <div className="mt-10 flex flex-wrap items-center justify-center gap-4">
          <Link
            href="/docs/getting-started"
            className="inline-flex h-11 items-center rounded-lg bg-primary px-6 font-medium text-primary-foreground text-sm transition-colors hover:bg-primary/90"
          >
            Get Started
          </Link>
          <Link
            href="/docs/api-reference"
            className="inline-flex h-11 items-center rounded-lg border border-border bg-background px-6 font-medium text-foreground text-sm transition-colors hover:bg-accent"
          >
            API Reference
          </Link>
        </div>
      </section>

      <section className="mx-auto w-full max-w-6xl px-6 py-16">
        <h2 className="mb-2 text-center font-bold text-2xl tracking-tight">
          Why Strait?
        </h2>
        <p className="mb-12 text-center text-muted-foreground">
          Everything you need for background job orchestration in one system.
        </p>
        <div className="grid gap-6 sm:grid-cols-2 lg:grid-cols-3">
          {features.map((feature) => (
            <div
              key={feature.title}
              className="rounded-lg border border-border bg-card p-6 transition-colors hover:bg-accent/50"
            >
              <h3 className="mb-2 font-semibold">{feature.title}</h3>
              <p className="text-muted-foreground text-sm">
                {feature.description}
              </p>
            </div>
          ))}
        </div>
      </section>

      <section className="mx-auto w-full max-w-6xl px-6 py-16">
        <h2 className="mb-2 text-center font-bold text-2xl tracking-tight">
          Explore the Docs
        </h2>
        <p className="mb-12 text-center text-muted-foreground">
          Jump into any section to start learning.
        </p>
        <div className="grid gap-6 sm:grid-cols-2 lg:grid-cols-3">
          {sections.map((section) => (
            <Link
              key={section.title}
              href={section.href}
              className="group rounded-lg border border-border bg-card p-6 transition-colors hover:border-primary/50 hover:bg-accent/50"
            >
              <h3 className="mb-2 font-semibold group-hover:text-primary">
                {section.title}
              </h3>
              <p className="mb-4 text-muted-foreground text-sm">
                {section.description}
              </p>
              <div className="flex flex-wrap gap-2">
                {section.items.map((item) => (
                  <span
                    key={item}
                    className="rounded-md bg-muted px-2 py-0.5 text-muted-foreground text-xs"
                  >
                    {item}
                  </span>
                ))}
              </div>
            </Link>
          ))}
        </div>
      </section>

      <section className="mx-auto w-full max-w-6xl px-6 py-16 text-center">
        <div className="rounded-lg border border-border bg-card p-12">
          <h2 className="mb-4 font-bold text-2xl tracking-tight">
            Ready to get started?
          </h2>
          <p className="mb-8 text-muted-foreground">
            Follow the quickstart guide to run your first job in under 10
            minutes.
          </p>
          <Link
            href="/docs/getting-started/quickstart"
            className="inline-flex h-11 items-center rounded-lg bg-primary px-8 font-medium text-primary-foreground text-sm transition-colors hover:bg-primary/90"
          >
            Quick Start Guide
          </Link>
        </div>
      </section>
    </main>
  );
}
