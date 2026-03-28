import { HugeiconsIcon } from "@hugeicons/react";
import { Input } from "@strait/ui/components/input";
import { Shell } from "@strait/ui/components/shell";
import { useQuery } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";
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

import { filterAgents } from "@/components/agents/agent-list-utils";
import ErrorComponent from "@/components/common/error-component";
import NoProjectState from "@/components/common/no-project-state";
import TableEmptyState from "@/components/common/table-empty-state";
import TablePageSkeleton from "@/components/common/table-page-skeleton";
import { agentColumns } from "@/components/tables/agents-columns";
import { DataTable } from "@/components/ui/data-table/data-table";
import type { Agent } from "@/hooks/api/types";
import { agentsQueryOptions } from "@/hooks/api/use-agents";
import { SearchIcon, SparklesIcon } from "@/lib/icons";
import type { AppRouteContext } from "@/routes/app/layout";

export const searchSchema = z.object({
  query: z.string().optional(),
});

export const Route = createFileRoute("/app/agents/")({
  validateSearch: zodValidator(searchSchema),
  loader: async ({ context }) => {
    const { session } = context as AppRouteContext;
    const hasProject = !!session.user.activeProjectId;
    if (hasProject) {
      await context.queryClient.ensureQueryData(agentsQueryOptions());
    }
    return { hasProject, session };
  },
  pendingComponent: TablePageSkeleton,
  errorComponent: ErrorComponent,
  component: AgentsPage,
});

function AgentsPage() {
  const { hasProject, session } = Route.useLoaderData();
  const search = Route.useSearch();
  const navigate = Route.useNavigate();
  const { data } = useQuery({
    ...agentsQueryOptions(),
    enabled: hasProject,
  });
  const agents = data as Agent[] | undefined;

  const filteredData = useMemo(
    () => filterAgents(hasProject ? (agents ?? []) : [], search.query),
    [agents, hasProject, search.query]
  );

  const [rowSelection, setRowSelection] = useState<Record<string, boolean>>({});

  const table = useReactTable({
    data: filteredData,
    columns: agentColumns,
    getCoreRowModel: getCoreRowModel(),
    getFilteredRowModel: getFilteredRowModel(),
    getSortedRowModel: getSortedRowModel(),
    getPaginationRowModel: getPaginationRowModel(),
    enableRowSelection: true,
    onRowSelectionChange: setRowSelection,
    state: { globalFilter: search.query ?? "", rowSelection },
    onGlobalFilterChange: (query) =>
      navigate({
        search: (prev) => ({ ...prev, query: query || undefined }),
      }),
    getRowId: (row) => row.id,
  });

  const emptyState = hasProject ? (
    <TableEmptyState
      description="No agents yet. Create and deploy your first managed agent from the API."
      hideButton
      icon={
        <HugeiconsIcon className="size-6 text-foreground" icon={SparklesIcon} />
      }
      title="No agents found"
    />
  ) : (
    <NoProjectState user={session.user} />
  );

  return (
    <Shell>
      <div className="flex items-center gap-3 pb-2.5">
        <div className="relative w-full max-w-[500px]">
          <HugeiconsIcon
            className="absolute top-1/2 left-3 -translate-y-1/2 text-muted-foreground"
            icon={SearchIcon}
            size={16}
          />
          <Input
            aria-label="Search agents"
            className="pl-9"
            onChange={(e) =>
              navigate({
                search: (prev) => ({
                  ...prev,
                  query: e.target.value || undefined,
                }),
              })
            }
            placeholder="Search agents by name, slug, or model..."
            value={search.query ?? ""}
          />
        </div>
      </div>

      <DataTable<Agent> emptyState={emptyState} table={table} />
    </Shell>
  );
}
