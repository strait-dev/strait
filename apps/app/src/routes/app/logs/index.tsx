import { HugeiconsIcon } from "@hugeicons/react";
import { Badge } from "@strait/ui/components/badge";
import { Input } from "@strait/ui/components/input";
import { Shell } from "@strait/ui/components/shell";
import { cn } from "@strait/ui/utils/index";
import { useSuspenseQuery } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";
import { zodValidator } from "@tanstack/zod-adapter";
import { formatDistanceToNow } from "date-fns";
import { useMemo, useState } from "react";
import { z } from "zod/v4";

import type { RunEvent } from "@/hooks/api/types";
import { eventsQueryOptions } from "@/hooks/api/use-events";
import { FileTextIcon, SearchIcon } from "@/lib/icons";

const LEVEL_STYLES: Record<string, { dot: string; badge: string }> = {
  info: {
    dot: "bg-info",
    badge: "bg-info/10 text-info border-info/20",
  },
  warn: {
    dot: "bg-warning",
    badge: "bg-warning/10 text-warning border-warning/20",
  },
  error: {
    dot: "bg-destructive",
    badge: "bg-destructive/10 text-destructive border-destructive/20",
  },
  debug: {
    dot: "bg-muted-foreground",
    badge:
      "bg-muted-foreground/10 text-muted-foreground border-muted-foreground/20",
  },
};

const searchSchema = z.object({
  query: z.string().optional(),
  page: z.number().optional().default(1),
});

export const Route = createFileRoute("/app/logs/")({
  validateSearch: zodValidator(searchSchema),
  loader: async ({ context }) => {
    await context.queryClient.ensureQueryData(
      eventsQueryOptions({ type: "log" })
    );
  },
  component: LogsPage,
});

function LogsPage() {
  const search = Route.useSearch();
  const navigate = Route.useNavigate();
  const { data } = useSuspenseQuery(
    eventsQueryOptions({ type: "log", page: search.page })
  );

  const [expandedId, setExpandedId] = useState<string | null>(null);

  const allLogs = useMemo(
    () => (data?.data ?? []).filter((e) => e.type === "log"),
    [data?.data]
  );

  const logs = useMemo(() => {
    if (!search.query) {
      return allLogs;
    }
    const q = search.query.toLowerCase();
    return allLogs.filter(
      (log) =>
        log.message.toLowerCase().includes(q) ||
        log.run_id.toLowerCase().includes(q)
    );
  }, [allLogs, search.query]);

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
            placeholder="Search logs by message or run ID..."
            value={search.query ?? ""}
          />
        </div>
      </div>

      {logs.length === 0 ? (
        <div className="flex flex-col items-center gap-2 py-16 text-center">
          <HugeiconsIcon className="size-6 text-primary" icon={FileTextIcon} />
          <p className="font-medium">No logs found</p>
          <p className="text-muted-foreground">
            No log entries match the current filters.
          </p>
        </div>
      ) : (
        <div className="overflow-hidden rounded-lg border border-border/70">
          <div className="divide-y divide-border/70">
            {logs.map((log) => (
              <LogRow
                expanded={expandedId === log.id}
                key={log.id}
                log={log}
                onToggle={() =>
                  setExpandedId((prev) => (prev === log.id ? null : log.id))
                }
              />
            ))}
          </div>
        </div>
      )}
    </Shell>
  );
}

function LogRow({
  log,
  expanded,
  onToggle,
}: {
  log: RunEvent;
  expanded: boolean;
  onToggle: () => void;
}) {
  const style = LEVEL_STYLES[log.level] ?? LEVEL_STYLES.info;

  return (
    // biome-ignore lint/a11y/useKeyWithClickEvents lint/a11y/noStaticElementInteractions lint/a11y/noNoninteractiveElementInteractions: log row toggle
    <div
      className="cursor-pointer px-4 py-3 transition-colors hover:bg-muted/40"
      onClick={onToggle}
    >
      <div className="flex items-center gap-3">
        <span className={cn("size-2 shrink-0 rounded-full", style.dot)} />
        <Badge className={cn("shrink-0", style.badge)} variant="outline">
          {log.level}
        </Badge>
        <span className="min-w-0 flex-1 truncate">{log.message}</span>
        <span className="shrink-0 font-mono text-muted-foreground text-xs">
          {log.run_id}
        </span>
        <span className="shrink-0 text-muted-foreground text-xs">
          {formatDistanceToNow(new Date(log.created_at), { addSuffix: true })}
        </span>
      </div>
      {expanded && log.data != null && (
        <pre className="mt-2 overflow-x-auto rounded-md bg-muted p-3 font-mono text-xs">
          {JSON.stringify(log.data, null, 2)}
        </pre>
      )}
    </div>
  );
}
