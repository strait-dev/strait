import { HugeiconsIcon } from "@hugeicons/react";
import { ActivityFeed } from "@strait/ui/components/activity-feed";
import { Button } from "@strait/ui/components/button";
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from "@strait/ui/components/empty";
import { Shell } from "@strait/ui/components/shell";
import {
  formatStatusLabel,
  StatusBadge,
} from "@strait/ui/components/status-badge";
import { useQuery } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";
import { zodValidator } from "@tanstack/zod-adapter";
import { z } from "zod/v4";
import ErrorComponent from "@/components/common/error-component";
import NoProjectState from "@/components/common/no-project-state";
import TablePageSkeleton from "@/components/common/table-page-skeleton";
import { usePageEvent } from "@/hooks/analytics/use-page-event";
import type { EventTrigger, PaginatedResponse } from "@/hooks/api/types";
import { eventsQueryOptions } from "@/hooks/api/use-events";
import { useCursorPagination } from "@/hooks/use-cursor-pagination";
import { ActivityIcon } from "@/lib/icons";
import { EVENT_STATUSES } from "@/lib/status";
import type { AppRouteContext } from "@/routes/app/layout";

export const searchSchema = z.object({
  status: z.string().optional(),
  cursor: z.string().optional(),
  perPage: z.coerce.number().optional(),
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
              <StatusBadge dotOnly size="xs" status={status} />
              {formatStatusLabel(status)}
            </Button>
          );
        })}
      </div>

      {/* Timeline */}
      {events.length === 0 ? (
        <Empty className="h-[300px]">
          <EmptyHeader>
            <EmptyMedia media="icon" size="lg">
              <HugeiconsIcon
                className="size-6 text-foreground"
                icon={ActivityIcon}
              />
            </EmptyMedia>
            <EmptyTitle>No events found</EmptyTitle>
            <EmptyDescription>
              Events will appear here after triggers are received for this
              project.
            </EmptyDescription>
          </EmptyHeader>
        </Empty>
      ) : (
        <ActivityFeed
          height="auto"
          items={events.map((event) => ({
            id: event.id,
            status: event.status,
            title: event.event_key,
            timestamp: event.requested_at,
            description: `${event.trigger_type} | ${event.source_type}`,
          }))}
        />
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
