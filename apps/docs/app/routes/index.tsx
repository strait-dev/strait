import { createFileRoute } from "@tanstack/react-router";
import { HomeLayout } from "fumadocs-ui/layouts/home";
import { Button } from "@strait/ui/components/button";
import { Badge } from "@strait/ui/components/badge";
import {
	Card,
	CardDescription,
	CardHeader,
	CardTitle,
} from "@strait/ui/components/card";
import { Separator } from "@strait/ui/components/separator";
import { TerminalDemo } from "@/app/components/terminal-demo";

export const Route = createFileRoute("/")({
	component: HomePage,
});

const GITHUB_REPO = "https://github.com/leonardomso/strait";

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

function HomePage() {
	return (
		<HomeLayout
			nav={{
				title: "Strait Docs",
				url: "/",
			}}
			links={[
				{ text: "Documentation", url: "/docs/getting-started" },
				{ text: "API Reference", url: "/docs/api-reference" },
				{ text: "SDKs", url: "/docs/sdks" },
				{
					text: "GitHub",
					url: GITHUB_REPO,
					external: true,
				},
			]}
			githubUrl={GITHUB_REPO}
		>
			<main className="flex flex-1 flex-col">
				<section className="relative flex flex-col items-center justify-center px-6 py-24 text-center">
					<div className="absolute inset-0 -z-10 bg-gradient-to-b from-primary/5 to-transparent" />
					<Badge variant="secondary" className="mb-4 uppercase tracking-widest">
						Documentation
					</Badge>
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
						<Button size="xl" // biome-ignore lint/a11y/useAnchorContent: content provided by Button children
						render={<a href="/docs/getting-started" />}>
							Get Started
						</Button>
						<Button
							variant="outline"
							size="xl"
							// biome-ignore lint/a11y/useAnchorContent: content provided by Button children
						render={<a href="/docs/api-reference" />}
						>
							API Reference
						</Button>
					</div>
					<TerminalDemo />
				</section>

				<Separator />

				<section className="mx-auto w-full max-w-6xl px-6 py-16">
					<h2 className="mb-2 text-center font-bold text-2xl tracking-tight">
						Why Strait?
					</h2>
					<p className="mb-12 text-center text-muted-foreground">
						Everything you need for background job orchestration in one system.
					</p>
					<div className="grid gap-6 sm:grid-cols-2 lg:grid-cols-3">
						{features.map((feature) => (
							<Card
								key={feature.title}
								className="transition-colors hover:bg-accent/50"
							>
								<CardHeader>
									<CardTitle>{feature.title}</CardTitle>
									<CardDescription>{feature.description}</CardDescription>
								</CardHeader>
							</Card>
						))}
					</div>
				</section>

				<Separator />

				<section className="mx-auto w-full max-w-6xl px-6 py-16">
					<h2 className="mb-2 text-center font-bold text-2xl tracking-tight">
						Explore the Docs
					</h2>
					<p className="mb-12 text-center text-muted-foreground">
						Jump into any section to start learning.
					</p>
					<div className="grid gap-6 sm:grid-cols-2 lg:grid-cols-3">
						{sections.map((section) => (
							<a key={section.title} href={section.href} className="group">
								<Card className="h-full transition-colors hover:border-primary/50 hover:bg-accent/50">
									<CardHeader>
										<CardTitle className="group-hover:text-primary">
											{section.title}
										</CardTitle>
										<CardDescription>{section.description}</CardDescription>
										<div className="flex flex-wrap gap-2 pt-2">
											{section.items.map((item) => (
												<Badge key={item} variant="secondary">
													{item}
												</Badge>
											))}
										</div>
									</CardHeader>
								</Card>
							</a>
						))}
					</div>
				</section>

				<Separator />

				<section className="mx-auto w-full max-w-6xl px-6 py-16 text-center">
					<Card className="p-12">
						<h2 className="mb-4 font-bold text-2xl tracking-tight">
							Ready to get started?
						</h2>
						<p className="mb-8 text-muted-foreground">
							Follow the quickstart guide to run your first job in under 10
							minutes.
						</p>
						<Button
							size="xl"
							// biome-ignore lint/a11y/useAnchorContent: content provided by Button children
						render={<a href="/docs/getting-started/quickstart" />}
						>
							Quick Start Guide
						</Button>
					</Card>
				</section>
			</main>
		</HomeLayout>
	);
}
