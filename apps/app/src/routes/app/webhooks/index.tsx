import { HugeiconsIcon } from "@hugeicons/react";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@strait/ui/components/alert-dialog";
import { Button } from "@strait/ui/components/button";
import {
  DataGrid,
  DataGridContainer,
  DataGridScrollArea,
  DataGridSelectionBar,
  DataGridTable,
} from "@strait/ui/components/data-grid";
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from "@strait/ui/components/empty";
import { InputWithStartIcon } from "@strait/ui/components/input-with-start-icon";
import { Shell } from "@strait/ui/components/shell";
import { StatusBadge } from "@strait/ui/components/status-badge";
import { useQuery } from "@tanstack/react-query";
import { createFileRoute, Link } from "@tanstack/react-router";
import {
  getCoreRowModel,
  getFilteredRowModel,
  getSortedRowModel,
  useReactTable,
} from "@tanstack/react-table";
import { zodValidator } from "@tanstack/zod-adapter";
import { useMemo, useState } from "react";
import { z } from "zod/v4";
import { CursorPagination } from "@/components/common/cursor-pagination";
import ErrorComponent from "@/components/common/error-component";
import { FacetedStatusFilter } from "@/components/common/faceted-status-filter";
import NoProjectState from "@/components/common/no-project-state";
import TablePageSkeleton from "@/components/common/table-page-skeleton";
import {
  getResourceTableInitialState,
  RESOURCE_TABLE_CLASS_NAMES,
} from "@/components/tables/resource-table";
import { createWebhookColumns } from "@/components/tables/webhooks-columns";
import { usePageEvent } from "@/hooks/analytics/use-page-event";
import type { PaginatedResponse, WebhookSubscription } from "@/hooks/api/types";
import {
  useDeleteWebhook,
  webhooksQueryOptions,
} from "@/hooks/api/use-webhooks";
import { useProjectPermissions } from "@/hooks/auth/use-project-permissions";
import { useCursorPagination } from "@/hooks/use-cursor-pagination";
import { useHydratedTableData } from "@/hooks/use-hydrated-table-data";
import {
  EyeIcon,
  PlusIcon,
  SearchIcon,
  TrashIcon,
  WebhookIcon,
} from "@/lib/icons";
import { webhookResourcePermissions } from "@/lib/resource-permissions";
import { WEBHOOK_STATUS_OPTIONS } from "@/lib/status";
import type { AppRouteContext } from "@/routes/app/layout";

const searchArraySchema = z.preprocess(
  (value) => (typeof value === "string" ? [value] : value),
  z.array(z.string()).optional()
);

export const searchSchema = z.object({
  query: z.string().optional(),
  status: searchArraySchema,
  cursor: z.string().optional(),
  perPage: z.coerce.number().optional(),
});

export const Route = createFileRoute("/app/webhooks/")({
  head: () => ({ meta: [{ title: "Webhooks · Strait" }] }),
  validateSearch: zodValidator(searchSchema),
  loaderDeps: ({ search }) => ({
    limit: search.perPage ?? 20,
    cursor: search.cursor,
  }),
  loader: async ({ context, deps }) => {
    const { session } = context as AppRouteContext;
    const hasProject = !!session.user.activeProjectId;
    if (hasProject) {
      await context.queryClient.ensureQueryData(
        webhooksQueryOptions({ limit: deps.limit, cursor: deps.cursor })
      );
    }
    return { hasProject, session };
  },
  pendingComponent: TablePageSkeleton,
  errorComponent: ErrorComponent,
  component: WebhooksPage,
});

function WebhooksPage() {
  usePageEvent("webhooks_viewed");
  const { hasProject, session } = Route.useLoaderData();
  const search = Route.useSearch();
  const navigate = Route.useNavigate();
  const pagination = useCursorPagination(
    { cursor: search.cursor, perPage: search.perPage },
    navigate
  );
  const { data } = useQuery({
    ...webhooksQueryOptions({
      limit: pagination.perPage,
      cursor: pagination.cursor,
    }),
    enabled: hasProject,
  });

  const selectedStatuses = search.status ?? [];

  const typed = data as PaginatedResponse<WebhookSubscription> | undefined;

  const filteredData = useMemo(() => {
    let webhooks = hasProject ? (typed?.data ?? []) : [];
    const query = search.query?.trim().toLowerCase();
    if (query) {
      webhooks = webhooks.filter((webhook) =>
        [
          webhook.id,
          webhook.webhook_url,
          webhook.event_types?.join(" "),
          webhook.created_at,
        ]
          .filter(Boolean)
          .some((value) => value?.toLowerCase().includes(query))
      );
    }
    if (selectedStatuses.length === 0) {
      return webhooks;
    }
    return webhooks.filter((webhook) => {
      if (selectedStatuses.includes("Active") && webhook.active) {
        return true;
      }
      if (selectedStatuses.includes("Inactive") && !webhook.active) {
        return true;
      }
      return false;
    });
  }, [typed, selectedStatuses, hasProject, search.query]);

  const deleteWebhook = useDeleteWebhook();
  const [deleteTarget, setDeleteTarget] = useState<string[] | null>(null);
  const { permissions } = useProjectPermissions(session.user.activeProjectId);
  const actionPermissions = webhookResourcePermissions(permissions);

  const [rowSelection, setRowSelection] = useState<Record<string, boolean>>({});
  const tableData = useHydratedTableData(filteredData);
  const table = useReactTable({
    data: tableData.data,
    columns: createWebhookColumns({
      onView: handleRowClick,
      onDelete: actionPermissions.canDelete
        ? (wh) => setDeleteTarget([wh.id])
        : undefined,
      disabled: !tableData.isHydrated,
    }),
    getCoreRowModel: getCoreRowModel(),
    getFilteredRowModel: getFilteredRowModel(),
    getSortedRowModel: getSortedRowModel(),
    manualPagination: true,
    enableRowSelection: true,
    initialState: getResourceTableInitialState(),
    onRowSelectionChange: setRowSelection,
    state: { globalFilter: search.query ?? "", rowSelection },
    onGlobalFilterChange: (query) =>
      navigate({
        search: (prev) => ({
          ...prev,
          query: query || undefined,
          cursor: undefined,
        }),
      }),
    getRowId: (row) => row.id,
  });

  const selectedIds = Object.keys(rowSelection).filter(
    (id) => rowSelection[id]
  );

  const summary = useMemo(() => {
    let active = 0;
    let inactive = 0;
    for (const webhook of filteredData) {
      if (webhook.active) {
        active++;
      } else {
        inactive++;
      }
    }
    return { active, inactive };
  }, [filteredData]);

  function handleStatusFiltersChange(statuses: string[]) {
    navigate({
      search: (prev) => ({
        ...prev,
        status: statuses.length > 0 ? statuses : undefined,
        cursor: undefined,
      }),
    });
  }

  function handleRowClick(webhook: WebhookSubscription) {
    navigate({ to: "/app/webhooks/$id", params: { id: webhook.id } });
  }

  const emptyState = hasProject ? (
    <Empty className="h-[300px]">
      <EmptyHeader>
        <EmptyMedia media="icon" size="lg">
          <HugeiconsIcon
            className="text-muted-foreground"
            icon={WebhookIcon}
            size={24}
          />
        </EmptyMedia>
        <EmptyTitle>No webhooks yet</EmptyTitle>
        <EmptyDescription>
          Create a webhook to receive notifications about run events.
        </EmptyDescription>
      </EmptyHeader>
    </Empty>
  ) : (
    <NoProjectState user={session.user} />
  );

  return (
    <Shell>
      <h1 className="sr-only">Webhooks</h1>
      {filteredData.length > 0 && (
        <div className="flex flex-wrap items-center gap-4 pb-3 text-sm">
          <span className="text-muted-foreground">
            {filteredData.length} subscription
            {filteredData.length === 1 ? "" : "s"}
          </span>
          <span className="flex items-center gap-1.5">
            <StatusBadge dotOnly size="xs" status="active" />
            <span className="tabular-nums">{summary.active}</span>
            <span className="text-muted-foreground">active</span>
          </span>
          <span className="flex items-center gap-1.5">
            <StatusBadge dotOnly size="xs" status="inactive" />
            <span className="tabular-nums">{summary.inactive}</span>
            <span className="text-muted-foreground">inactive</span>
          </span>
        </div>
      )}

      <div className="flex items-center gap-3 pb-2.5">
        <InputWithStartIcon
          aria-label="Search"
          containerClassName="w-full max-w-[500px]"
          icon={<HugeiconsIcon icon={SearchIcon} size={16} />}
          onChange={(e) =>
            navigate({
              search: (prev) => ({
                ...prev,
                query: e.target.value || undefined,
                cursor: undefined,
              }),
            })
          }
          placeholder="Search webhooks"
          value={search.query ?? ""}
        />

        <FacetedStatusFilter
          onChange={handleStatusFiltersChange}
          options={WEBHOOK_STATUS_OPTIONS.map((status) => ({
            label: status,
            value: status,
          }))}
          values={selectedStatuses}
        />

        {actionPermissions.canCreate && (
          <Button
            className="ml-auto"
            disabled={!hasProject}
            render={<Link to="/app/webhooks/new" />}
          >
            <HugeiconsIcon className="mr-1.5" icon={PlusIcon} size={14} />
            Create webhook
          </Button>
        )}
      </div>

      <div>
        <DataGrid
          emptyMessage={emptyState}
          loading={tableData.isLoading}
          onRowClick={handleRowClick}
          recordCount={
            tableData.isHydrated ? table.getRowModel().rows.length : 0
          }
          table={table}
          tableClassNames={RESOURCE_TABLE_CLASS_NAMES}
        >
          <DataGridContainer>
            <DataGridScrollArea>
              <DataGridTable />
            </DataGridScrollArea>
          </DataGridContainer>
          <CursorPagination
            cursor={{
              pageSize: pagination.perPage,
              hasMore: typed?.has_more ?? false,
              canGoBack: pagination.canGoBack,
              onNext: () => {
                if (typed?.next_cursor) {
                  pagination.goNext(typed.next_cursor);
                }
              },
              onPrev: pagination.goPrev,
              onPageSizeChange: pagination.setPerPage,
            }}
            table={table}
          />
          <DataGridSelectionBar
            actions={[
              ...(selectedIds.length === 1
                ? [
                    {
                      label: "View",
                      icon: EyeIcon,
                      onClick: () => {
                        const webhook = table
                          .getRowModel()
                          .rows.find((r) => r.id === selectedIds[0])?.original;
                        if (webhook) {
                          handleRowClick(webhook);
                        }
                      },
                    },
                  ]
                : []),
              ...(actionPermissions.canDelete
                ? [
                    {
                      label: "Delete",
                      icon: TrashIcon,
                      onClick: () => setDeleteTarget(selectedIds),
                      variant: "destructive" as const,
                    },
                  ]
                : []),
            ]}
          />
        </DataGrid>
      </div>

      <AlertDialog
        onOpenChange={(open) => {
          if (!open) {
            setDeleteTarget(null);
          }
        }}
        open={!!deleteTarget}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>
              Delete{" "}
              {deleteTarget?.length === 1
                ? "webhook"
                : `${deleteTarget?.length} webhooks`}
              ?
            </AlertDialogTitle>
            <AlertDialogDescription>
              This will permanently delete the selected webhook
              {deleteTarget && deleteTarget.length > 1 ? "s" : ""}. Deliveries
              in progress will not be affected.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction
              onClick={() => {
                if (deleteTarget) {
                  for (const id of deleteTarget) {
                    deleteWebhook.mutate(id);
                  }
                }
                setDeleteTarget(null);
                setRowSelection({});
              }}
            >
              Delete
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </Shell>
  );
}
