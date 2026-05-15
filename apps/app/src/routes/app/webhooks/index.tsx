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
import { Badge } from "@strait/ui/components/badge";
import { Button } from "@strait/ui/components/button";
import {
  DropdownMenu,
  DropdownMenuCheckboxItem,
  DropdownMenuContent,
  DropdownMenuTrigger,
} from "@strait/ui/components/dropdown-menu";
import { Input } from "@strait/ui/components/input";
import { Shell } from "@strait/ui/components/shell";
import { useQuery } from "@tanstack/react-query";
import { createFileRoute, Link } from "@tanstack/react-router";
import {
  getCoreRowModel,
  getFilteredRowModel,
  getPaginationRowModel,
  getSortedRowModel,
  useReactTable,
} from "@tanstack/react-table";
import { zodValidator } from "@tanstack/zod-adapter";
import { useMemo, useState } from "react";
import { z } from "zod/v4";
import ErrorComponent from "@/components/common/error-component";
import NoProjectState from "@/components/common/no-project-state";
import TableEmptyState from "@/components/common/table-empty-state";
import TablePageSkeleton from "@/components/common/table-page-skeleton";
import { createWebhookColumns } from "@/components/tables/webhooks-columns";
import { DataTable } from "@/components/ui/data-table/data-table";
import { DataTableFloatingBar } from "@/components/ui/data-table/data-table-floating-bar";
import { usePageEvent } from "@/hooks/analytics/use-page-event";
import type { WebhookSubscription } from "@/hooks/api/types";
import {
  useDeleteWebhook,
  webhooksQueryOptions,
} from "@/hooks/api/use-webhooks";
import {
  EyeIcon,
  FilterIcon,
  PlusIcon,
  SearchIcon,
  TrashIcon,
  WebhookIcon,
} from "@/lib/icons";
import { WEBHOOK_STATUS_OPTIONS } from "@/lib/status";
import type { AppRouteContext } from "@/routes/app/layout";

export const searchSchema = z.object({
  query: z.string().optional(),
  status: z.array(z.string()).optional(),
  page: z.number().optional().default(1),
});

export const Route = createFileRoute("/app/webhooks/")({
  head: () => ({ meta: [{ title: "Webhooks · Strait" }] }),
  validateSearch: zodValidator(searchSchema),
  loader: async ({ context }) => {
    const { session } = context as AppRouteContext;
    const hasProject = !!session.user.activeProjectId;
    if (hasProject) {
      await context.queryClient.ensureQueryData(webhooksQueryOptions());
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
  const { data } = useQuery({
    ...webhooksQueryOptions(),
    enabled: hasProject,
  });

  const selectedStatuses = search.status ?? [];

  const filteredData = useMemo(() => {
    const webhooks = hasProject ? (data?.data ?? []) : [];
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
  }, [data?.data, selectedStatuses, hasProject]);

  const deleteWebhook = useDeleteWebhook();
  const [deleteTarget, setDeleteTarget] = useState<string[] | null>(null);

  const [rowSelection, setRowSelection] = useState<Record<string, boolean>>({});
  const table = useReactTable({
    data: filteredData,
    columns: createWebhookColumns({
      onDelete: (wh) => setDeleteTarget([wh.id]),
    }),
    getCoreRowModel: getCoreRowModel(),
    getFilteredRowModel: getFilteredRowModel(),
    getSortedRowModel: getSortedRowModel(),
    getPaginationRowModel: getPaginationRowModel(),
    enableRowSelection: true,
    onRowSelectionChange: setRowSelection,
    state: { globalFilter: search.query ?? "", rowSelection },
    onGlobalFilterChange: (query) =>
      navigate({
        search: (prev) => ({ ...prev, query: query || undefined, page: 1 }),
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

  function toggleStatus(status: string) {
    const current = new Set(selectedStatuses);
    if (current.has(status)) {
      current.delete(status);
    } else {
      current.add(status);
    }
    const arr = Array.from(current);
    navigate({
      search: (prev) => ({
        ...prev,
        status: arr.length > 0 ? arr : undefined,
        page: 1,
      }),
    });
  }

  function handleRowClick(webhook: WebhookSubscription) {
    navigate({ to: "/app/webhooks/$id", params: { id: webhook.id } });
  }

  const emptyState = hasProject ? (
    <TableEmptyState
      description="Create a webhook to receive notifications about run events."
      hideButton
      icon={
        <HugeiconsIcon
          className="text-muted-foreground"
          icon={WebhookIcon}
          size={24}
        />
      }
      title="No webhooks yet"
    />
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
            <span className="inline-block size-1.5 rounded-full bg-success" />
            <span className="tabular-nums">{summary.active}</span>
            <span className="text-muted-foreground">active</span>
          </span>
          <span className="flex items-center gap-1.5">
            <span className="inline-block size-1.5 rounded-full bg-muted-foreground/40" />
            <span className="tabular-nums">{summary.inactive}</span>
            <span className="text-muted-foreground">inactive</span>
          </span>
        </div>
      )}

      <div className="flex items-center gap-3 pb-2.5">
        <div className="relative w-full max-w-[500px]">
          <HugeiconsIcon
            className="absolute top-1/2 left-3 -translate-y-1/2 text-muted-foreground"
            icon={SearchIcon}
            size={16}
          />
          <Input
            aria-label="Search"
            className="pl-9"
            onChange={(e) =>
              navigate({
                search: (prev) => ({
                  ...prev,
                  query: e.target.value || undefined,
                  page: 1,
                }),
              })
            }
            placeholder="Search webhooks..."
            value={search.query ?? ""}
          />
        </div>

        <DropdownMenu>
          <DropdownMenuTrigger render={<Button variant="outline" />}>
            <HugeiconsIcon className="mr-1.5" icon={FilterIcon} size={14} />
            Status
            {selectedStatuses.length > 0 && (
              <Badge variant="default">{selectedStatuses.length}</Badge>
            )}
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end" className="w-48">
            {WEBHOOK_STATUS_OPTIONS.map((status) => (
              <DropdownMenuCheckboxItem
                checked={selectedStatuses.includes(status)}
                key={status}
                onCheckedChange={() => toggleStatus(status)}
              >
                {status}
              </DropdownMenuCheckboxItem>
            ))}
          </DropdownMenuContent>
        </DropdownMenu>

        <Button
          className="ml-auto"
          disabled={!hasProject}
          render={<Link to="/app/webhooks/new" />}
        >
          <HugeiconsIcon className="mr-1.5" icon={PlusIcon} size={14} />
          Create webhook
        </Button>
      </div>

      {/* biome-ignore lint/a11y/useKeyWithClickEvents lint/a11y/noNoninteractiveElementInteractions lint/a11y/noStaticElementInteractions: event delegation on table container */}
      <div
        className="[&_tbody_tr]:cursor-pointer"
        onClick={(e) => {
          const target = e.target as HTMLElement;
          if (target.closest("a, button")) {
            return;
          }
          const row = target.closest("tr[data-row-index]");
          if (!row) {
            return;
          }
          const idx = Number(row.getAttribute("data-row-index"));
          const webhook = table.getRowModel().rows[idx]?.original;
          if (webhook) {
            handleRowClick(webhook);
          }
        }}
      >
        <DataTable
          ariaLabel="Webhooks"
          emptyState={emptyState}
          floatingBar={
            <DataTableFloatingBar
              actions={[
                ...(selectedIds.length === 1
                  ? [
                      {
                        label: "View",
                        icon: EyeIcon,
                        onClick: () => {
                          const webhook = table
                            .getRowModel()
                            .rows.find(
                              (r) => r.id === selectedIds[0]
                            )?.original;
                          if (webhook) {
                            handleRowClick(webhook);
                          }
                        },
                      },
                    ]
                  : []),
                {
                  label: "Delete",
                  icon: TrashIcon,
                  onClick: () => setDeleteTarget(selectedIds),
                  variant: "destructive" as const,
                },
              ]}
              onClearSelection={() => setRowSelection({})}
              selectedCount={selectedIds.length}
            />
          }
          table={table}
        />
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
