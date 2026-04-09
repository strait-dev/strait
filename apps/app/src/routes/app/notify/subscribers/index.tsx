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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@strait/ui/components/select";
import { Shell } from "@strait/ui/components/shell";
import { Switch } from "@strait/ui/components/switch";
import { toast } from "@strait/ui/components/toast";
import { useQuery } from "@tanstack/react-query";
import { createFileRoute, Link } from "@tanstack/react-router";
import {
  type ColumnDef,
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
import NotifyStatusBadge from "@/components/notify/notify-status-badge";
import { createSelectColumn } from "@/components/tables/shared-columns";
import { DataTable } from "@/components/ui/data-table/data-table";
import type {
  NotifyDigestPolicy,
  NotifySubscriber,
  NotifySubscriberStatus,
} from "@/hooks/api/types";
import {
  notifySubscribersQueryOptions,
  notifyTopicsQueryOptions,
  useAddNotifyTopicSubscriber,
  useCreateNotifySubscriber,
  useRemoveNotifyTopicSubscriber,
  useUpdateNotifySubscriberPreference,
} from "@/hooks/api/use-notify";
import { FilterIcon, MailIcon, PlusIcon, SearchIcon } from "@/lib/icons";
import {
  notifyCursorPageLimit,
  resolveNotifyNextCursor,
} from "@/lib/notify-cursor";
import { notifyDigestPolicyOptions } from "@/lib/notify-preferences";
import { NOTIFY_SUBSCRIBER_STATUS_OPTIONS } from "@/lib/status";
import type { AppRouteContext } from "@/routes/app/layout";

export const searchSchema = z.object({
  query: z.string().optional(),
  status: z.array(z.string()).optional(),
  tenant_id: z.string().optional(),
});

type NotifySubscribersSearch = z.infer<typeof searchSchema>;

export const Route = createFileRoute("/app/notify/subscribers/")({
  validateSearch: zodValidator(searchSchema),
  loader: async ({ context }) => {
    const { session } = context as AppRouteContext;
    const hasProject = !!session.user.activeProjectId;
    if (hasProject) {
      await Promise.all([
        context.queryClient.ensureQueryData(
          notifySubscribersQueryOptions({ limit: notifyCursorPageLimit })
        ),
        context.queryClient.ensureQueryData(notifyTopicsQueryOptions()),
      ]);
    }
    return { hasProject, session };
  },
  pendingComponent: TablePageSkeleton,
  errorComponent: ErrorComponent,
  component: NotifySubscribersPage,
});

const columns: ColumnDef<NotifySubscriber>[] = [
  createSelectColumn<NotifySubscriber>(),
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
  {
    id: "actions",
    header: "Actions",
    cell: ({ row }) => (
      <Button
        render={
          <Link
            params={{ id: row.original.id }}
            to="/app/notify/subscribers/$id"
          />
        }
        size="sm"
        variant="outline"
      >
        View
      </Button>
    ),
  },
];

function NotifySubscribersPage() {
  const { hasProject, session } = Route.useLoaderData();
  const search = Route.useSearch();
  const navigate = Route.useNavigate();

  const [newExternalID, setNewExternalID] = useState("");
  const [newEmail, setNewEmail] = useState("");

  const [rowSelection, setRowSelection] = useState<Record<string, boolean>>({});
  const [bulkTopicKey, setBulkTopicKey] = useState("");
  const [bulkPreferenceScope, setBulkPreferenceScope] = useState("global");
  const [bulkDigestPolicy, setBulkDigestPolicy] =
    useState<NotifyDigestPolicy>("instant");
  const [bulkEmailEnabled, setBulkEmailEnabled] = useState(true);
  const [bulkInboxEnabled, setBulkInboxEnabled] = useState(true);

  const createSubscriber = useCreateNotifySubscriber();
  const addTopicSubscriber = useAddNotifyTopicSubscriber();
  const removeTopicSubscriber = useRemoveNotifyTopicSubscriber();
  const updatePreference = useUpdateNotifySubscriberPreference();

  const [cursor, setCursor] = useState<string>();
  const [cursorHistory, setCursorHistory] = useState<(string | undefined)[]>(
    []
  );

  const selectedStatuses = search.status ?? [];

  const subscribersQuery = useQuery({
    ...notifySubscribersQueryOptions({
      status:
        selectedStatuses.length === 1
          ? (selectedStatuses[0] as NotifySubscriberStatus)
          : undefined,
      tenant_id: search.tenant_id,
      limit: notifyCursorPageLimit,
      cursor,
    }),
    enabled: hasProject,
  });

  const topicsQuery = useQuery({
    ...notifyTopicsQueryOptions(),
    enabled: hasProject,
  });

  const pageItems = subscribersQuery.data ?? [];
  const topics = topicsQuery.data ?? [];
  const sortedTopics = [...topics].sort((a, b) =>
    a.topic_key.localeCompare(b.topic_key)
  );
  const nextCursor = resolveNotifyNextCursor(pageItems, notifyCursorPageLimit);

  const filtered = useMemo(() => {
    return pageItems.filter((subscriber) => {
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
  }, [search.query, selectedStatuses, pageItems]);

  const table = useReactTable({
    data: filtered,
    columns,
    getCoreRowModel: getCoreRowModel(),
    getFilteredRowModel: getFilteredRowModel(),
    getSortedRowModel: getSortedRowModel(),
    getPaginationRowModel: getPaginationRowModel(),
    enableRowSelection: true,
    onRowSelectionChange: setRowSelection,
    getRowId: (row) => row.id,
    state: { rowSelection },
  });

  if (!hasProject) {
    return (
      <Shell>
        <NoProjectState user={session.user} />
      </Shell>
    );
  }

  const selectedSubscriberIDs = table
    .getFilteredSelectedRowModel()
    .rows.map((row) => row.original.id);

  const updateFilters = (
    updater: (prev: NotifySubscribersSearch) => NotifySubscribersSearch
  ) => {
    setCursor(undefined);
    setCursorHistory([]);
    navigate({
      search: (prev) => updater(prev),
    });
  };

  const toggleStatus = (status: NotifySubscriberStatus) => {
    const next = new Set(selectedStatuses);
    if (next.has(status)) {
      next.delete(status);
    } else {
      next.add(status);
    }
    const values = Array.from(next);
    updateFilters((prev) => ({
      ...prev,
      status: values.length > 0 ? values : undefined,
    }));
  };

  const goToNextPage = () => {
    if (!nextCursor) {
      return;
    }

    setCursorHistory((history) => [...history, cursor]);
    setCursor(nextCursor);
  };

  const goToPreviousPage = () => {
    setCursorHistory((history) => {
      if (history.length === 0) {
        return history;
      }

      const nextHistory = [...history];
      const previousCursor = nextHistory.pop();
      setCursor(previousCursor);
      return nextHistory;
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

  const applyBulkTopicMembership = async (action: "add" | "remove") => {
    if (selectedSubscriberIDs.length === 0) {
      toast.error("Select at least one subscriber");
      return;
    }
    if (!bulkTopicKey.trim()) {
      toast.error("Topic key is required for bulk topic membership updates");
      return;
    }

    const run = async () => {
      for (const subscriberID of selectedSubscriberIDs) {
        if (action === "add") {
          await addTopicSubscriber.mutateAsync({
            topicKey: bulkTopicKey,
            subscriber_id: subscriberID,
          });
        } else {
          await removeTopicSubscriber.mutateAsync({
            topicKey: bulkTopicKey,
            subscriberId: subscriberID,
          });
        }
      }
    };

    await toast.promise(run(), {
      loading:
        action === "add"
          ? "Adding subscribers to topic..."
          : "Removing subscribers from topic...",
      success:
        action === "add"
          ? "Subscribers added to topic"
          : "Subscribers removed from topic",
      error:
        action === "add"
          ? "Failed to add subscribers to topic"
          : "Failed to remove subscribers from topic",
    });

    setRowSelection({});
  };

  const applyBulkPreferenceUpdate = async () => {
    if (selectedSubscriberIDs.length === 0) {
      toast.error("Select at least one subscriber");
      return;
    }
    if (!bulkPreferenceScope.trim()) {
      toast.error("Preference scope is required");
      return;
    }

    const run = async () => {
      for (const subscriberID of selectedSubscriberIDs) {
        await updatePreference.mutateAsync({
          subscriberId: subscriberID,
          scope: bulkPreferenceScope.trim(),
          channel_prefs: {
            email: bulkEmailEnabled,
            inbox: bulkInboxEnabled,
          },
          digest_policy: bulkDigestPolicy,
        });
      }
    };

    await toast.promise(run(), {
      loading: "Applying preference updates...",
      success: "Preferences updated for selected subscribers",
      error: "Failed to update preferences for selected subscribers",
    });

    setRowSelection({});
  };

  const isCreateSubscriberDisabled =
    createSubscriber.isPending || !newExternalID.trim();
  const isBulkWorking =
    addTopicSubscriber.isPending ||
    removeTopicSubscriber.isPending ||
    updatePreference.isPending;

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
            <Button disabled={isCreateSubscriberDisabled} onClick={create}>
              <HugeiconsIcon className="mr-1.5 size-4" icon={PlusIcon} />
              Create
            </Button>
          </div>
        </CardContent>
      </Card>

      <div className="mb-3 flex flex-wrap items-center gap-3">
        <div className="relative w-full max-w-[360px]">
          <Input
            className="pl-9"
            onChange={(event) => {
              updateFilters((prev) => ({
                ...prev,
                query: event.target.value || undefined,
              }));
            }}
            placeholder="Search subscriber"
            value={search.query ?? ""}
          />
          <div className="pointer-events-none absolute top-1/2 left-3 -translate-y-1/2 text-muted-foreground">
            <HugeiconsIcon className="size-4" icon={SearchIcon} />
          </div>
        </div>

        <Input
          className="w-full max-w-[220px]"
          onChange={(event) => {
            updateFilters((prev) => ({
              ...prev,
              tenant_id: event.target.value || undefined,
            }));
          }}
          placeholder="Tenant ID"
          value={search.tenant_id ?? ""}
        />

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

      <Card className="mb-4">
        <CardHeader>
          <CardTitle className="text-sm">Bulk actions</CardTitle>
          <CardDescription>
            Apply topic membership or preference updates to selected rows only.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-3">
          <p className="text-muted-foreground text-xs">
            {selectedSubscriberIDs.length} selected
          </p>

          <div className="grid gap-3 md:grid-cols-[1fr_auto_auto]">
            <Select
              onValueChange={(value) => setBulkTopicKey(value ?? "")}
              value={bulkTopicKey || undefined}
            >
              <SelectTrigger>
                <SelectValue placeholder="Select topic for bulk membership" />
              </SelectTrigger>
              <SelectContent>
                {sortedTopics.map((topic) => (
                  <SelectItem key={topic.id} value={topic.topic_key}>
                    {topic.topic_key}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <Button
              disabled={isBulkWorking || selectedSubscriberIDs.length === 0}
              onClick={() => applyBulkTopicMembership("add")}
            >
              Bulk add to topic
            </Button>
            <Button
              disabled={isBulkWorking || selectedSubscriberIDs.length === 0}
              onClick={() => applyBulkTopicMembership("remove")}
              variant="outline"
            >
              Bulk remove from topic
            </Button>
          </div>

          <div className="grid gap-3 md:grid-cols-4">
            <Input
              onChange={(event) => setBulkPreferenceScope(event.target.value)}
              placeholder="Preference scope"
              value={bulkPreferenceScope}
            />
            <Select
              onValueChange={(value) =>
                setBulkDigestPolicy(value as NotifyDigestPolicy)
              }
              value={bulkDigestPolicy}
            >
              <SelectTrigger>
                <SelectValue placeholder="Digest policy" />
              </SelectTrigger>
              <SelectContent>
                {notifyDigestPolicyOptions.map((option) => (
                  <SelectItem key={option} value={option}>
                    {option}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <div className="flex items-center justify-between rounded-md border p-2">
              <span className="text-sm">Email</span>
              <Switch
                checked={bulkEmailEnabled}
                onCheckedChange={setBulkEmailEnabled}
              />
            </div>
            <div className="flex items-center justify-between rounded-md border p-2">
              <span className="text-sm">Inbox</span>
              <Switch
                checked={bulkInboxEnabled}
                onCheckedChange={setBulkInboxEnabled}
              />
            </div>
          </div>

          <Button
            disabled={isBulkWorking || selectedSubscriberIDs.length === 0}
            onClick={applyBulkPreferenceUpdate}
            variant="secondary"
          >
            Bulk apply preferences
          </Button>
        </CardContent>
      </Card>

      <DataTable
        emptyState={
          <TableEmptyState
            description="No subscribers found for this project."
            hideButton
            icon={
              <HugeiconsIcon
                className="size-6 text-foreground"
                icon={MailIcon}
              />
            }
            title="No subscribers"
          />
        }
        table={table}
      />

      <div className="mt-3 flex items-center justify-between gap-2">
        <p className="text-muted-foreground text-xs">
          Showing up to {notifyCursorPageLimit} subscribers per page.
        </p>
        <div className="flex gap-2">
          <Button
            aria-label="Go to previous subscribers page"
            disabled={cursorHistory.length === 0 || subscribersQuery.isFetching}
            onClick={goToPreviousPage}
            variant="outline"
          >
            Previous page
          </Button>
          <Button
            aria-label="Go to next subscribers page"
            disabled={!nextCursor || subscribersQuery.isFetching}
            onClick={goToNextPage}
            variant="outline"
          >
            Next page
          </Button>
        </div>
      </div>
    </Shell>
  );
}
