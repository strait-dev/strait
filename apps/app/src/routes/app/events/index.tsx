import { Button } from "@strait/ui/components/button";
import { Shell } from "@strait/ui/components/shell";
import { cn } from "@strait/ui/utils";
import { useQuery } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";
import { zodValidator } from "@tanstack/zod-adapter";
import { z } from "zod/v4";
import ErrorComponent from "@/components/common/error-component";
import NoProjectState from "@/components/common/no-project-state";
import TablePageSkeleton from "@/components/common/table-page-skeleton";
import EventRow from "@/components/events/event-row";
import { usePageEvent } from "@/hooks/analytics/use-page-event";
import type { EventTrigger, PaginatedResponse } from "@/hooks/api/types";
import { eventsQueryOptions } from "@/hooks/api/use-events";
import { useCursorPagination } from "@/hooks/use-cursor-pagination";
import { EVENT_STATUS_STYLES, EVENT_STATUSES } from "@/lib/status";
import type { AppRouteContext } from "@/routes/app/layout";

export const searchSchema = z.object({
  status: z.string().optional(),
  cursor: z.string().optional(),
  perPage: z.number().optional(),
});

export const Route = createFileRoute("/app/events/")({
  head: () => ({ meta: [{ title: "Events · Strait" }] }),
  validateSearch: zodValidator(searchSchema),
  loaderDeps: ({ search }) => ({
    limit: search.perPage ?? 20,
    cursor: search.cursor,
    status: search.status,
  }),
  loader: async ({ context, deps }) => {
    const { session } = context as AppRouteContext;
    const hasProject = !!session.user.activeProjectId;
    if (hasProject) {
      await context.queryClient.ensureQueryData(
        eventsQueryOptions({
          limit: deps.limit,
          cursor: deps.cursor,
          status: deps.status,
        })
      );
    }
    return { hasProject, session };
  },
  pendingComponent: TablePageSkeleton,
  errorComponent: ErrorComponent,
  component: EventsPage,
});

function EventsPage() {
  usePageEvent("events_viewed");
  const { hasProject, session } = Route.useLoaderData();
  const search = Route.useSearch();
  const navigate = Route.useNavigate();
  const pagination = useCursorPagination(
    { cursor: search.cursor, perPage: search.perPage },
    navigate
  );
  const { data } = useQuery({
    ...eventsQueryOptions({
      limit: pagination.perPage,
      cursor: pagination.cursor,
      status: search.status,
    }),
    enabled: hasProject,
  });

  const typed = data as PaginatedResponse<EventTrigger> | undefined;
  const events = hasProject ? (typed?.data ?? []) : [];

  if (!hasProject) {
    return (
      <Shell>
        <h1 className="sr-only">Events</h1>
        <NoProjectState user={session.user} />
      </Shell>
    );
  }

  return (
    <Shell>
      <h1 className="sr-only">Events</h1>
      {/* Status filter */}
      <div className="flex items-center gap-2 pb-2.5">
        <Button
          onClick={() =>
            navigate({
              search: (prev) => ({
                ...prev,
                status: undefined,
                cursor: undefined,
              }),
            })
          }
          variant={search.status ? "ghost" : "secondary"}
        >
          All
        </Button>
        {EVENT_STATUSES.map((status) => {
          const style = EVENT_STATUS_STYLES[status];
          const active = search.status === status;
          return (
            <Button
              key={status}
              onClick={() =>
                navigate({
                  search: (prev) => ({
                    ...prev,
                    status: active ? undefined : status,
                    cursor: undefined,
                  }),
                })
              }
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

      {/* Pagination controls */}
      {(pagination.canGoBack || typed?.has_more) && (
        <div className="flex items-center justify-between pt-4">
          <Button
            disabled={!pagination.canGoBack}
            onClick={pagination.goPrev}
            variant="outline"
          >
            Previous
          </Button>
          <Button
            disabled={!typed?.has_more}
            onClick={() => {
              if (typed?.next_cursor) {
                pagination.goNext(typed.next_cursor);
              }
            }}
            variant="outline"
          >
            Next
          </Button>
        </div>
      )}
    </Shell>
  );
}
