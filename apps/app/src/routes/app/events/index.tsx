import { Badge } from "@strait/ui/components/badge";
import { Button } from "@strait/ui/components/button";
import { Shell } from "@strait/ui/components/shell";
import { cn } from "@strait/ui/utils/index";
import { useSuspenseQuery } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";
import { zodValidator } from "@tanstack/zod-adapter";
import { formatDistanceToNow } from "date-fns";
import { z } from "zod/v4";

import ErrorComponent from "@/components/common/error-component";
import { NoProjectState } from "@/components/common/no-project-state";
import { TablePageSkeleton } from "@/components/common/table-page-skeleton";
import type { EventTrigger } from "@/hooks/api/types";
import { eventsQueryOptions } from "@/hooks/api/use-events";
import type { AuthUser } from "@/routes/__root";

const STATUS_STYLES: Record<
  string,
  { dot: string; label: string; badge: string }
> = {
  pending: {
    dot: "bg-chart-3",
    label: "Pending",
    badge: "bg-chart-3/10 text-chart-3 border-chart-3/20",
  },
  received: {
    dot: "bg-chart-2",
    label: "Received",
    badge: "bg-chart-2/10 text-chart-2 border-chart-2/20",
  },
  expired: {
    dot: "bg-chart-5",
    label: "Expired",
    badge: "bg-chart-5/10 text-chart-5 border-chart-5/20",
  },
  failed: {
    dot: "bg-destructive",
    label: "Failed",
    badge: "bg-destructive/10 text-destructive border-destructive/20",
  },
  canceled: {
    dot: "bg-muted-foreground",
    label: "Canceled",
    badge:
      "bg-muted-foreground/10 text-muted-foreground border-muted-foreground/20",
  },
};

const EVENT_STATUSES = Object.keys(STATUS_STYLES);

const searchSchema = z.object({
  status: z.string().optional(),
  page: z.number().optional().default(1),
});

export const Route = createFileRoute("/app/events/")({
  validateSearch: zodValidator(searchSchema),
  loader: async ({ context }) => {
    const session = (context as unknown as { session: { user: AuthUser } })
      .session;
    const hasProject = !!session?.user?.activeProjectId;
    if (hasProject) {
      await context.queryClient.ensureQueryData(eventsQueryOptions());
    }
    return { hasProject };
  },
  pendingComponent: TablePageSkeleton,
  errorComponent: ErrorComponent,
  component: EventsPage,
});

function EventsPage() {
  const { hasProject } = Route.useLoaderData() as { hasProject: boolean };
  const { session } = Route.useRouteContext() as any;
  if (!hasProject) {
    return (
      <Shell>
        <NoProjectState user={session.user} />
      </Shell>
    );
  }

  return <EventsPageContent />;
}

function EventsPageContent() {
  const search = Route.useSearch();
  const navigate = Route.useNavigate();
  const { data } = useSuspenseQuery(eventsQueryOptions());

  const events = data?.data ?? [];

  return (
    <Shell>
      {/* Status filter */}
      <div className="flex items-center gap-2 pb-2.5">
        <Button
          onClick={() =>
            navigate({
              search: (prev) => ({ ...prev, status: undefined, page: 1 }),
            })
          }
          size="sm"
          variant={search.status ? "ghost" : "secondary"}
        >
          All
        </Button>
        {EVENT_STATUSES.map((status) => {
          const style = STATUS_STYLES[status];
          const active = search.status === status;
          return (
            <Button
              key={status}
              onClick={() =>
                navigate({
                  search: (prev) => ({
                    ...prev,
                    status: active ? undefined : status,
                    page: 1,
                  }),
                })
              }
              size="sm"
              variant={active ? "secondary" : "ghost"}
            >
              <span
                className={cn(
                  "mr-1.5 inline-block size-2 rounded-full",
                  style.dot
                )}
              />
              {style.label}
            </Button>
          );
        })}
      </div>

      {/* Timeline */}
      {events.length === 0 ? (
        <div className="py-12 text-center text-muted-foreground">
          No events found.
        </div>
      ) : (
        <div className="relative space-y-0">
          {/* Vertical line */}
          <div className="absolute top-0 bottom-0 left-[11px] w-px bg-border" />

          {events.map((event) => (
            <EventRow event={event} key={event.id} />
          ))}
        </div>
      )}
    </Shell>
  );
}

function EventRow({ event }: { event: EventTrigger }) {
  const style = STATUS_STYLES[event.status] ?? STATUS_STYLES.pending;

  return (
    <div className="relative flex items-start gap-3 py-2.5 pl-0">
      {/* Dot */}
      <span
        className={cn(
          "relative z-10 mt-1.5 h-[9px] w-[9px] shrink-0 rounded-full border-2 border-background",
          style.dot
        )}
      />

      {/* Content */}
      <div className="flex min-w-0 flex-1 flex-col gap-0.5">
        <div className="flex items-center gap-2">
          <Badge className={cn("px-1.5 py-0", style.badge)} variant="outline">
            {style.label}
          </Badge>
          <span className="font-mono text-muted-foreground text-xs">
            {formatDistanceToNow(new Date(event.requested_at), {
              addSuffix: true,
            })}
          </span>
        </div>
        <p className="text-sm">{event.event_key}</p>
        <span className="font-mono text-muted-foreground text-xs">
          {event.trigger_type} | {event.source_type}
        </span>
      </div>
    </div>
  );
}
