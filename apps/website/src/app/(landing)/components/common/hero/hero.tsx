import { ArrowRight02Icon } from "@hugeicons/core-free-icons";
import { HugeiconsIcon } from "@hugeicons/react";
import { Button } from "@strait/ui/components/button";
import Link from "next/link";
import Reveal from "@/components/landing/reveal.tsx";
import {
  StaggerGroup,
  StaggerItem,
} from "@/components/landing/stagger-group.tsx";
import Shell from "@/components/layout/shell.tsx";
import MockBrowserWindow from "@/components/magicui/mock-browser-window.tsx";
import { dashboardHref } from "@/lib/urls.ts";

const Hero = () => (
  <section className="relative isolate overflow-hidden pt-32 pb-16 sm:pt-40 sm:pb-24">
    <div className="parallax-slow absolute inset-0 -z-10 bg-[linear-gradient(to_bottom,_var(--primary)/0.06,_transparent_40%)]" />
    <div className="orchestration-grid absolute inset-0 -z-10 opacity-[0.14]" />
    <div className="absolute inset-0 -z-10 bg-[linear-gradient(to_bottom,_transparent,_var(--background)_70%)]" />
    <div className="paper-texture absolute inset-0 -z-10 opacity-[0.02]" />

    <Shell variant="wide">
      {/* Centered text */}
      <div className="mx-auto flex max-w-4xl flex-col items-center text-center">
        <Reveal delay={0}>
          <span className="kicker">OPEN SOURCE JOB ORCHESTRATION</span>
        </Reveal>

        <Reveal delay={0.1} variant="blur">
          <h1 className="mt-6 text-balance text-4xl leading-[1.12] tracking-[-0.025em] sm:text-5xl lg:text-6xl">
            <span className="text-foreground">
              The workflow engine for your backend.
            </span>{" "}
            <span className="text-muted-foreground">
              Queues, retries, DAGs, and observability — powered by Postgres. No
              broker required.
            </span>
          </h1>
        </Reveal>

        <Reveal delay={0.2} spring>
          <p className="mt-5 max-w-2xl text-pretty text-base text-muted-foreground/70 leading-relaxed sm:mt-6 sm:text-lg">
            Define workflows in TypeScript, Go, or Python. Strait handles
            retries, dead letters, approval gates, and cost budgets so your team
            ships faster and sleeps better.
          </p>
        </Reveal>

        <Reveal delay={0.3} spring>
          <div className="mt-10 flex flex-col items-center gap-4 sm:flex-row">
            <Button
              className="transition-shadow duration-300"
              render={<Link href={dashboardHref("/login")} />}
              variant="gradient"
            >
              Start building free
              <HugeiconsIcon className="size-4" icon={ArrowRight02Icon} />
            </Button>
            <Button render={<Link href="/docs/quickstart" />} variant="ghost">
              Read the docs
              <HugeiconsIcon className="size-4" icon={ArrowRight02Icon} />
            </Button>
          </div>
        </Reveal>

        <StaggerGroup
          className="mt-6 flex flex-wrap items-center justify-center gap-2.5"
          delay={0.06}
        >
          <StaggerItem>
            <span className="rounded-full border border-border/60 bg-card px-3 py-1 text-muted-foreground text-sm">
              Built on Postgres
            </span>
          </StaggerItem>
          <StaggerItem>
            <span className="rounded-full border border-border/60 bg-card px-3 py-1 text-muted-foreground text-sm">
              Full lifecycle tracking
            </span>
          </StaggerItem>
          <StaggerItem>
            <span className="rounded-full border border-border/60 bg-card px-3 py-1 text-muted-foreground text-sm">
              Apache 2.0 licensed
            </span>
          </StaggerItem>
        </StaggerGroup>
      </div>

      {/* Product screenshot */}
      <Reveal className="mt-16 sm:mt-20" delay={0.4} spring variant="scale">
        <MockBrowserWindow
          className="shadow-2xl shadow-black/10"
          url="app.trystrait.ai/workflows"
        >
          <div className="bg-card p-4 sm:p-6">
            {/* Sidebar + main content mock */}
            <div className="flex gap-4">
              {/* Sidebar */}
              <div className="hidden w-48 shrink-0 space-y-3 border-border/40 border-r pr-4 md:block">
                <div className="h-2.5 w-20 rounded bg-foreground/10" />
                <div className="space-y-2 pt-2">
                  {["Workflows", "Runs", "Jobs", "Queues", "Events"].map(
                    (item) => (
                      <div
                        className={`rounded-md px-2.5 py-1.5 text-xs ${item === "Workflows" ? "bg-primary/10 text-foreground" : "text-muted-foreground"}`}
                        key={item}
                      >
                        {item}
                      </div>
                    )
                  )}
                </div>
              </div>

              {/* Main content */}
              <div className="min-w-0 flex-1 space-y-4">
                {/* Header bar */}
                <div className="flex items-center justify-between">
                  <div className="flex items-center gap-2">
                    <div className="h-3 w-24 rounded bg-foreground/15" />
                    <span className="rounded-full bg-success/15 px-2 py-0.5 text-[10px] text-success">
                      Active
                    </span>
                  </div>
                  <div className="flex gap-2">
                    <div className="h-6 w-16 rounded-md bg-muted" />
                    <div className="h-6 w-20 rounded-md bg-primary/80" />
                  </div>
                </div>

                {/* Workflow DAG */}
                <div className="rounded-xl border border-border/40 bg-background/50 p-4">
                  <div className="flex items-center justify-center gap-3 overflow-x-auto py-2 sm:gap-5">
                    {[
                      { label: "Validate", status: "completed" },
                      { label: "Enrich", status: "completed" },
                      { label: "Process", status: "executing" },
                      { label: "Notify", status: "queued" },
                    ].map((node, i) => {
                      const statusClasses: Record<string, string> = {
                        completed:
                          "border-success/30 bg-success/8 text-success",
                        executing:
                          "border-foreground/20 bg-foreground/5 text-foreground",
                        queued:
                          "border-border/60 bg-muted/30 text-muted-foreground",
                      };
                      return (
                        <div
                          className="flex items-center gap-3 sm:gap-5"
                          key={node.label}
                        >
                          <div
                            className={`rounded-lg border px-3 py-1.5 text-center text-xs sm:px-4 sm:py-2 ${statusClasses[node.status] ?? ""}`}
                          >
                            {node.label}
                            {node.status === "executing" && (
                              <span className="ml-1.5 inline-block size-1.5 animate-pulse rounded-full bg-foreground" />
                            )}
                          </div>
                          {i < 3 && (
                            <svg
                              className="hidden size-4 text-border sm:block"
                              fill="none"
                              viewBox="0 0 16 16"
                            >
                              <path
                                d="M3 8h10M10 5l3 3-3 3"
                                stroke="currentColor"
                                strokeLinecap="round"
                                strokeLinejoin="round"
                                strokeWidth={1.5}
                              />
                            </svg>
                          )}
                        </div>
                      );
                    })}
                  </div>
                </div>

                {/* Stats row */}
                <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
                  {[
                    { label: "Total Runs", value: "12,847" },
                    { label: "Success Rate", value: "99.2%" },
                    { label: "Avg Duration", value: "1.4s" },
                    { label: "Active Workers", value: "8" },
                  ].map((stat) => (
                    <div
                      className="rounded-lg border border-border/40 bg-background/50 p-3"
                      key={stat.label}
                    >
                      <p className="text-[10px] text-muted-foreground">
                        {stat.label}
                      </p>
                      <p className="mt-0.5 font-semibold text-foreground text-sm tabular-nums">
                        {stat.value}
                      </p>
                    </div>
                  ))}
                </div>

                {/* Recent runs table */}
                <div className="rounded-xl border border-border/40 bg-background/50">
                  <div className="border-border/40 border-b px-3 py-2">
                    <p className="text-muted-foreground text-xs">Recent Runs</p>
                  </div>
                  <div className="divide-y divide-border/30">
                    {[
                      {
                        name: "checkout-flow",
                        status: "completed",
                        time: "2s ago",
                        duration: "1.2s",
                      },
                      {
                        name: "data-sync",
                        status: "executing",
                        time: "5s ago",
                        duration: "—",
                      },
                      {
                        name: "send-reports",
                        status: "completed",
                        time: "12s ago",
                        duration: "3.8s",
                      },
                      {
                        name: "process-order",
                        status: "completed",
                        time: "18s ago",
                        duration: "0.9s",
                      },
                    ].map((run) => (
                      <div
                        className="flex items-center justify-between px-3 py-2"
                        key={run.name}
                      >
                        <div className="flex items-center gap-2">
                          <div
                            className={`size-1.5 rounded-full ${
                              run.status === "completed"
                                ? "bg-success"
                                : "animate-pulse bg-foreground"
                            }`}
                          />
                          <span className="font-mono text-foreground text-xs">
                            {run.name}
                          </span>
                        </div>
                        <div className="flex items-center gap-4">
                          <span className="text-[10px] text-muted-foreground tabular-nums">
                            {run.duration}
                          </span>
                          <span className="text-[10px] text-muted-foreground/60">
                            {run.time}
                          </span>
                        </div>
                      </div>
                    ))}
                  </div>
                </div>
              </div>
            </div>
          </div>
        </MockBrowserWindow>
      </Reveal>
    </Shell>
  </section>
);

export default Hero;
