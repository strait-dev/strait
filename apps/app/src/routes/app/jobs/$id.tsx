import { HugeiconsIcon } from "@hugeicons/react";
import { Badge } from "@strait/ui/components/badge.tsx";
import {
  Breadcrumb,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator,
} from "@strait/ui/components/breadcrumb.tsx";
import { Button } from "@strait/ui/components/button.tsx";
import { Shell } from "@strait/ui/components/shell.tsx";
import {
  Tabs,
  TabsContent,
  TabsList,
  TabsTrigger,
} from "@strait/ui/components/tabs.tsx";
import { useSuspenseQuery } from "@tanstack/react-query";
import { createFileRoute, Link } from "@tanstack/react-router";
import { useState } from "react";
import { StatusBadge } from "@/components/dashboard/status-badge.tsx";
import type { Job } from "@/hooks/api/types.ts";
import { jobQueryOptions } from "@/hooks/api/use-jobs.ts";
import {
  ClockIcon,
  GlobeIcon,
  PauseActionIcon,
  PlayActionIcon,
  RefreshIcon,
  TagIcon,
} from "@/lib/icons.ts";

export const Route = createFileRoute("/app/jobs/$id")({
  loader: async ({ context, params }) => {
    await context.queryClient.ensureQueryData(jobQueryOptions(params.id));
  },
  component: JobDetailPage,
});

function JobDetailPage() {
  const { id } = Route.useParams();
  const { data: job } = useSuspenseQuery(jobQueryOptions(id)) as {
    data: Job | null;
  };
  const [activeTab, setActiveTab] = useState("overview");

  if (!job) {
    return (
      <Shell>
        <div className="flex items-center justify-center py-20">
          <p className="text-muted-foreground">Job not found.</p>
        </div>
      </Shell>
    );
  }

  return (
    <Shell>
      {/* Breadcrumb */}
      <Breadcrumb>
        <BreadcrumbList>
          <BreadcrumbItem>
            <BreadcrumbLink>
              <Link to="/app/jobs">Jobs</Link>
            </BreadcrumbLink>
          </BreadcrumbItem>
          <BreadcrumbSeparator />
          <BreadcrumbItem>
            <BreadcrumbPage>{job.name}</BreadcrumbPage>
          </BreadcrumbItem>
        </BreadcrumbList>
      </Breadcrumb>

      {/* Header */}
      <div className="flex items-start justify-between pt-4 pb-6">
        <div className="flex flex-col gap-2">
          <div className="flex items-center gap-3">
            <h1 className="font-semibold text-2xl tracking-tight">
              {job.name}
            </h1>
            <StatusBadge
              showDot
              status={job.enabled ? "completed" : "paused"}
            />
          </div>
          {job.description && (
            <p className="text-muted-foreground text-sm">{job.description}</p>
          )}
        </div>
        <div className="flex gap-2">
          <Button size="sm">
            <HugeiconsIcon className="mr-1.5" icon={PlayActionIcon} size={14} />
            Trigger
          </Button>
          <Button size="sm" variant="outline">
            <HugeiconsIcon
              className="mr-1.5"
              icon={job.enabled ? PauseActionIcon : PlayActionIcon}
              size={14}
            />
            {job.enabled ? "Pause" : "Resume"}
          </Button>
        </div>
      </div>

      {/* Tabs */}
      <Tabs className="w-full" onValueChange={setActiveTab} value={activeTab}>
        <TabsList>
          <TabsTrigger value="overview">Overview</TabsTrigger>
          <TabsTrigger value="runs">Recent Runs</TabsTrigger>
          <TabsTrigger value="settings">Settings</TabsTrigger>
        </TabsList>

        <TabsContent className="mt-6 space-y-6" value="overview">
          {/* Stats grid */}
          <div className="grid grid-cols-3 gap-4">
            <StatCard label="Success Rate" value="98.2%" />
            <StatCard label="Total Runs" value="1,247" />
            <StatCard label="Avg Duration" value="4.2s" />
          </div>

          {/* Configuration */}
          <div className="space-y-3 rounded-md border p-4">
            <h3 className="font-medium text-muted-foreground text-xs uppercase">
              Configuration
            </h3>
            <div className="space-y-2.5">
              <ConfigRow
                icon={GlobeIcon}
                label="Endpoint"
                value={job.endpoint_url || "-"}
              />
              <ConfigRow
                icon={ClockIcon}
                label="Schedule"
                value={job.cron || "Manual"}
              />
              <ConfigRow
                icon={RefreshIcon}
                label="Retry"
                value={`${job.max_attempts} attempts (${job.retry_strategy})`}
              />
              <ConfigRow
                icon={ClockIcon}
                label="Timeout"
                value={`${job.timeout_secs}s`}
              />
            </div>
          </div>

          {/* Tags */}
          {job.tags && Object.keys(job.tags).length > 0 && (
            <div className="rounded-md border p-4">
              <h3 className="mb-3 flex items-center gap-1.5 font-medium text-muted-foreground text-xs uppercase">
                <HugeiconsIcon icon={TagIcon} size={12} />
                Tags
              </h3>
              <div className="flex flex-wrap gap-1.5">
                {Object.entries(job.tags).map(([key, val]) => (
                  <Badge className="text-xs" key={key} variant="secondary">
                    {key}: {String(val)}
                  </Badge>
                ))}
              </div>
            </div>
          )}
        </TabsContent>

        <TabsContent className="mt-6" value="runs">
          <div className="space-y-2">
            {/* Placeholder recent runs */}
            {[
              { id: "run_1", status: "completed" as const, time: "2m ago" },
              { id: "run_2", status: "completed" as const, time: "1h ago" },
              { id: "run_3", status: "failed" as const, time: "3h ago" },
            ].map((run) => (
              <div
                className="flex items-center justify-between rounded-md border px-4 py-3"
                key={run.id}
              >
                <span className="font-mono text-xs">{run.id}</span>
                <div className="flex items-center gap-3">
                  <StatusBadge size="xs" status={run.status} />
                  <span className="text-[11px] text-muted-foreground">
                    {run.time}
                  </span>
                </div>
              </div>
            ))}
          </div>
        </TabsContent>

        <TabsContent className="mt-6" value="settings">
          <div className="rounded-md border p-6">
            <h3 className="font-medium text-sm">Job Settings</h3>
            <p className="mt-1 text-muted-foreground text-xs">
              Configuration management for this job will be available soon.
            </p>
          </div>
        </TabsContent>
      </Tabs>
    </Shell>
  );
}

function StatCard({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-md border p-4 text-center">
      <p className="font-bold text-2xl">{value}</p>
      <p className="text-muted-foreground text-xs">{label}</p>
    </div>
  );
}

function ConfigRow({
  icon,
  label,
  value,
}: {
  icon: any;
  label: string;
  value: string;
}) {
  return (
    <div className="flex items-center gap-2 text-sm">
      <HugeiconsIcon className="text-muted-foreground" icon={icon} size={14} />
      <span className="text-muted-foreground">{label}</span>
      <span className="ml-auto font-mono text-xs">{value}</span>
    </div>
  );
}
