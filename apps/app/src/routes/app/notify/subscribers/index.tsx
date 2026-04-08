import { HugeiconsIcon } from "@hugeicons/react";
import { Badge } from "@strait/ui/components/badge";
import { Button } from "@strait/ui/components/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@strait/ui/components/card";
import {
  DropdownMenu,
  DropdownMenuCheckboxItem,
  DropdownMenuContent,
  DropdownMenuTrigger,
} from "@strait/ui/components/dropdown-menu";
import { Input } from "@strait/ui/components/input";
import { Shell } from "@strait/ui/components/shell";
import { toast } from "@strait/ui/components/toast";
import { useQuery } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";
import {
  getCoreRowModel,
  getFilteredRowModel,
  getPaginationRowModel,
  getSortedRowModel,
  useReactTable,
  type ColumnDef,
} from "@tanstack/react-table";
import { zodValidator } from "@tanstack/zod-adapter";
import { useMemo, useState } from "react";
import { z } from "zod/v4";
import ErrorComponent from "@/components/common/error-component";
import NoProjectState from "@/components/common/no-project-state";
import TableEmptyState from "@/components/common/table-empty-state";
import TablePageSkeleton from "@/components/common/table-page-skeleton";
import NotifyStatusBadge from "@/components/notify/notify-status-badge";
import { DataTable } from "@/components/ui/data-table/data-table";
import {
  notifySubscribersQueryOptions,
  useCreateNotifySubscriber,
} from "@/hooks/api/use-notify";
import type { NotifySubscriber } from "@/hooks/api/types";
import {
  FilterIcon,
  MailIcon,
  PlusIcon,
  SearchIcon,
} from "@/lib/icons";
import { NOTIFY_SUBSCRIBER_STATUS_OPTIONS } from "@/lib/status";
import type { AppRouteContext } from "@/routes/app/layout";

export const searchSchema = z.object({
  query: z.string().optional(),
  status: z.array(z.string()).optional(),
});

export const Route = createFileRoute("/app/notify/subscribers/")({
  validateSearch: zodValidator(searchSchema),
  loader: async ({ context }) => {
    const { session } = context as AppRouteContext;
    const hasProject = !!session.user.activeProjectId;
    if (hasProject) {
      await context.queryClient.ensureQueryData(notifySubscribersQueryOptions());
    }
    return { hasProject, session };
  },
  pendingComponent: TablePageSkeleton,
  errorComponent: ErrorComponent,
  component: NotifySubscribersPage,
});

const columns: ColumnDef<NotifySubscriber>[] = [
  {
    accessorKey: "external_id",
    header: "External ID",
    cell: ({ row }) => row.original.external_id,
  },
  {
    accessorKey: "email",
    header: "Email",
    cell: ({ row }) => row.original.email || "-",
  },
  {
    accessorKey: "tenant_id",
    header: "Tenant",
    cell: ({ row }) => row.original.tenant_id || "-",
  },
  {
    accessorKey: "status",
    header: "Status",
    cell: ({ row }) => <NotifyStatusBadge status={row.original.status} />,
  },
  {
    accessorKey: "created_at",
    header: "Created",
    cell: ({ row }) => new Date(row.original.created_at).toLocaleString(),
  },
];

function NotifySubscribersPage() {
  const { hasProject, session } = Route.useLoaderData();
  const search = Route.useSearch();
  const navigate = Route.useNavigate();

  const [newExternalID, setNewExternalID] = useState("");
  const [newEmail, setNewEmail] = useState("");

  const createSubscriber = useCreateNotifySubscriber();

  const subscribersQuery = useQuery({
    ...notifySubscribersQueryOptions(),
    enabled: hasProject,
  });

  const selectedStatuses = search.status ?? [];

  const filtered = useMemo(() => {
    const items = subscribersQuery.data ?? [];
    return items.filter((subscriber) => {
      if (
        selectedStatuses.length > 0 &&
        !selectedStatuses.includes(subscriber.status)
      ) {
        return false;
      }
      if (!search.query) {
        return true;
      }
      const q = search.query.toLowerCase();
      return (
        subscriber.external_id.toLowerCase().includes(q) ||
        (subscriber.email || "").toLowerCase().includes(q)
      );
    });
  }, [search.query, selectedStatuses, subscribersQuery.data]);

  const table = useReactTable({
    data: filtered,
    columns,
    getCoreRowModel: getCoreRowModel(),
    getFilteredRowModel: getFilteredRowModel(),
    getSortedRowModel: getSortedRowModel(),
    getPaginationRowModel: getPaginationRowModel(),
  });

  if (!hasProject) {
    return (
      <Shell>
        <NoProjectState user={session.user} />
      </Shell>
    );
  }

  const toggleStatus = (status: string) => {
    const next = new Set(selectedStatuses);
    if (next.has(status)) {
      next.delete(status);
    } else {
      next.add(status);
    }
    const values = Array.from(next);
    navigate({
      search: (prev) => ({
        ...prev,
        status: values.length > 0 ? values : undefined,
      }),
    });
  };

  const create = async () => {
    if (!newExternalID.trim()) {
      toast.error("External ID is required");
      return;
    }

    await toast.promise(
      createSubscriber.mutateAsync({
        external_id: newExternalID.trim(),
        email: newEmail.trim() || undefined,
      }),
      {
        loading: "Creating subscriber...",
        success: "Subscriber created",
        error: "Failed to create subscriber",
      }
    );

    setNewExternalID("");
    setNewEmail("");
  };

  return (
    <Shell>
      <Card className="mb-4">
        <CardHeader>
          <CardTitle className="text-sm">Create subscriber</CardTitle>
          <CardDescription>
            Add a recipient to use in Notify deliveries and topic memberships.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div className="grid gap-3 md:grid-cols-[1fr_1fr_auto]">
            <Input
              onChange={(event) => setNewExternalID(event.target.value)}
              placeholder="external-id"
              value={newExternalID}
            />
            <Input
              onChange={(event) => setNewEmail(event.target.value)}
              placeholder="email@example.com"
              value={newEmail}
            />
            <Button disabled={createSubscriber.isPending} onClick={create}>
              <HugeiconsIcon className="mr-1.5 size-4" icon={PlusIcon} />
              Create
            </Button>
          </div>
        </CardContent>
      </Card>

      <div className="mb-3 flex items-center gap-3">
        <div className="relative w-full max-w-[420px]">
          <Input
            className="pl-9"
            onChange={(event) =>
              navigate({
                search: (prev) => ({
                  ...prev,
                  query: event.target.value || undefined,
                }),
              })
            }
            placeholder="Search subscriber"
            value={search.query ?? ""}
          />
          <div className="pointer-events-none absolute top-1/2 left-3 -translate-y-1/2 text-muted-foreground">
            <HugeiconsIcon className="size-4" icon={SearchIcon} />
          </div>
        </div>

        <DropdownMenu>
          <DropdownMenuTrigger render={<Button variant="outline" />}>
            <HugeiconsIcon className="mr-1.5 size-4" icon={FilterIcon} />
            Status
            {selectedStatuses.length > 0 && (
              <Badge className="ml-2" variant="default">
                {selectedStatuses.length}
              </Badge>
            )}
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end" className="w-56">
            {NOTIFY_SUBSCRIBER_STATUS_OPTIONS.map((status) => (
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
      </div>

      {/* biome-ignore lint/a11y/useKeyWithClickEvents lint/a11y/noNoninteractiveElementInteractions lint/a11y/noStaticElementInteractions: event delegation on table rows */}
      <div
        className="[&_tbody_tr]:cursor-pointer"
        onClick={(event) => {
          const target = event.target as HTMLElement;
          if (target.closest("a, button")) {
            return;
          }

          const row = target.closest("tr[data-row-index]");
          if (!row) {
            return;
          }

          const index = Number(row.getAttribute("data-row-index"));
          const subscriber = table.getRowModel().rows[index]?.original;
          if (!subscriber) {
            return;
          }

          navigate({
            to: "/app/notify/subscribers/$id",
            params: { id: subscriber.id },
          });
        }}
      >
        <DataTable
          emptyState={
            <TableEmptyState
              description="No subscribers found for this project."
              hideButton
              icon={<HugeiconsIcon className="size-6 text-foreground" icon={MailIcon} />}
              title="No subscribers"
            />
          }
          table={table}
        />
      </div>
    </Shell>
  );
}
