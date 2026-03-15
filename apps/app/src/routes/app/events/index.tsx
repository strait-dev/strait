import { Badge } from "@strait/ui/components/badge";
import { Button } from "@strait/ui/components/button";
import { Shell } from "@strait/ui/components/shell";
import { cn } from "@strait/ui/utils/index";
import { useSuspenseQuery } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";
import { zodValidator } from "@tanstack/zod-adapter";
import { formatDistanceToNow } from "date-fns";
import { z } from "zod/v4";
import PageHeader from "@/components/common/page-header";
import type { RunEvent } from "@/hooks/api/types";
import { eventsQueryOptions } from "@/hooks/api/use-events";

const EVENT_TYPES = ["log", "state_change", "error", "progress"] as const;

const TYPE_STYLES: Record<
  string,
  { dot: string; label: string; badge: string }
> = {
  log: {
    dot: "bg-chart-2",
    label: "Log",
    badge: "bg-chart-2/10 text-chart-2 border-chart-2/20",
  },
  state_change: {
    dot: "bg-chart-5",
    label: "State Change",
    badge: "bg-chart-5/10 text-chart-5 border-chart-5/20",
  },
  error: {
    dot: "bg-destructive",
    label: "Error",
    badge: "bg-destructive/10 text-destructive border-destructive/20",
  },
  progress: {
    dot: "bg-chart-3",
    label: "Progress",
    badge: "bg-chart-3/10 text-chart-3 border-chart-3/20",
  },
};

const searchSchema = z.object({
  type: z.string().optional(),
  page: z.number().optional().default(1),
});

export const Route = createFileRoute("/app/events/")({
  validateSearch: zodValidator(searchSchema),
  loader: async ({ context }) => {
    await context.queryClient.ensureQueryData(eventsQueryOptions());
  },
  component: EventsPage,
});

function EventsPage() {
  const search = Route.useSearch();
  const navigate = Route.useNavigate();
  const { data } = useSuspenseQuery(
    eventsQueryOptions({ type: search.type, page: search.page })
  );

  const events = data?.data ?? [];

  return (
    <Shell>
      <PageHeader
        text="Timeline of run events across all jobs."
        title="Events"
      />

      {/* Type filter */}
      <div className="flex items-center gap-2 pt-4 pb-2">
        <Button
          onClick={() =>
            navigate({
              search: (prev) => ({ ...prev, type: undefined, page: 1 }),
            })
          }
          size="sm"
          variant={search.type ? "ghost" : "secondary"}
        >
          All
        </Button>
        {EVENT_TYPES.map((type) => {
          const style = TYPE_STYLES[type];
          const active = search.type === type;
          return (
            <Button
              key={type}
              onClick={() =>
                navigate({
                  search: (prev) => ({
                    ...prev,
                    type: active ? undefined : type,
                    page: 1,
                  }),
                })
              }
              size="sm"
              variant={active ? "secondary" : "ghost"}
            >
              <span
                className={cn(
                  "mr-1.5 inline-block h-2 w-2 rounded-full",
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

function EventRow({ event }: { event: RunEvent }) {
  const style = TYPE_STYLES[event.type] ?? TYPE_STYLES.log;

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
          <Badge
            className={cn("px-1.5 py-0 text-xs", style.badge)}
            variant="outline"
          >
            {style.label}
          </Badge>
          <span className="font-mono text-muted-foreground text-xs">
            {formatDistanceToNow(new Date(event.created_at), {
              addSuffix: true,
            })}
          </span>
        </div>
        <p
          className={cn(
            "text-sm",
            event.type === "error" && "text-destructive"
          )}
        >
          {event.message}
        </p>
        <span className="font-mono text-muted-foreground text-xs">
          {event.run_id}
        </span>
      </div>
    </div>
  );
}
