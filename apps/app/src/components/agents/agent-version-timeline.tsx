import { Badge } from "@strait/ui/components/badge";
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@strait/ui/components/card";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@strait/ui/components/select";
import { cn } from "@strait/ui/utils/index";
import { useState } from "react";
import ConfigDiff from "@/components/agents/config-diff";
import type { AgentVersion } from "@/hooks/api/use-agents";

type AgentVersionTimelineProps = {
  versions: AgentVersion[];
};

const STATUS_VARIANT: Record<string, "default" | "secondary" | "destructive"> =
  {
    deployed: "default",
    failed: "destructive",
    pending: "secondary",
  };

function formatTimestamp(value: string | undefined): string {
  if (!value) {
    return "-";
  }
  return new Date(value).toLocaleString();
}

function emptyConfig(): Record<string, object> {
  return {};
}

function VersionEntry({
  index,
  isLatest,
  previousConfig,
  version,
}: {
  index: number;
  isLatest: boolean;
  previousConfig: Record<string, object>;
  version: AgentVersion;
}) {
  const [expanded, setExpanded] = useState(false);
  const config = version.config_snapshot ?? emptyConfig();
  const isFirst = index === 0;

  return (
    <div className="relative flex gap-4 pb-6">
      <div className="flex flex-col items-center">
        <div
          className={cn(
            "flex h-8 w-8 shrink-0 items-center justify-center rounded-full border-2 font-bold text-xs",
            isLatest
              ? "border-primary bg-primary text-primary-foreground"
              : "border-muted-foreground/30 bg-card text-muted-foreground"
          )}
        >
          v{version.version}
        </div>
        {!isFirst && <div className="mt-1 w-px grow bg-muted-foreground/20" />}
      </div>
      <div className="flex-1 pt-1">
        <div className="flex items-center gap-2">
          <Badge variant={STATUS_VARIANT[version.status] ?? "secondary"}>
            {version.status}
          </Badge>
          {isLatest && (
            <Badge className="text-[10px]" variant="outline">
              current
            </Badge>
          )}
          <span className="text-muted-foreground text-xs">
            {formatTimestamp(version.deployed_at ?? version.created_at)}
          </span>
        </div>
        <div className="mt-1 flex items-center gap-3 text-muted-foreground text-xs">
          <span>Provider: {version.provider}</span>
          {version.created_by && <span>By: {version.created_by}</span>}
        </div>
        {!isFirst && (
          <button
            className="mt-2 text-primary text-xs underline underline-offset-2"
            onClick={() => setExpanded((prev) => !prev)}
            type="button"
          >
            {expanded ? "Hide diff" : "Show config diff"}
          </button>
        )}
        {expanded && !isFirst && (
          <div className="mt-2 rounded-md border bg-card p-3">
            <ConfigDiff
              left={previousConfig as Record<string, unknown>}
              right={config as Record<string, unknown>}
            />
          </div>
        )}
      </div>
    </div>
  );
}

function VersionCompare({ versions }: { versions: AgentVersion[] }) {
  const [leftIdx, setLeftIdx] = useState<string>("0");
  const [rightIdx, setRightIdx] = useState<string>(
    String(Math.min(1, versions.length - 1))
  );

  const leftVersion = versions[Number(leftIdx)];
  const rightVersion = versions[Number(rightIdx)];

  if (!(leftVersion && rightVersion)) {
    return null;
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-sm">Compare Versions</CardTitle>
      </CardHeader>
      <CardContent>
        <div className="mb-4 flex items-center gap-4">
          <Select
            onValueChange={(v) => {
              if (v) {
                setLeftIdx(v);
              }
            }}
            value={leftIdx}
          >
            <SelectTrigger className="w-32">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {versions.map((v, i) => (
                <SelectItem key={v.id} value={String(i)}>
                  v{v.version}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <span className="text-muted-foreground text-sm">vs</span>
          <Select
            onValueChange={(v) => {
              if (v) {
                setRightIdx(v);
              }
            }}
            value={rightIdx}
          >
            <SelectTrigger className="w-32">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {versions.map((v, i) => (
                <SelectItem key={v.id} value={String(i)}>
                  v{v.version}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        <ConfigDiff
          left={
            (leftVersion.config_snapshot ?? emptyConfig()) as Record<
              string,
              unknown
            >
          }
          right={
            (rightVersion.config_snapshot ?? emptyConfig()) as Record<
              string,
              unknown
            >
          }
        />
      </CardContent>
    </Card>
  );
}

export default function AgentVersionTimeline({
  versions,
}: AgentVersionTimelineProps) {
  if (versions.length === 0) {
    return (
      <p className="py-8 text-center text-muted-foreground text-sm">
        No deployments yet. Deploy your agent to see version history.
      </p>
    );
  }

  // Versions are sorted newest-first from the API.
  const sorted = [...versions].sort((a, b) => b.version - a.version);

  return (
    <div className="space-y-6">
      <div className="space-y-0">
        {sorted.map((version, index) => {
          const previousVersion = sorted[index + 1];
          return (
            <VersionEntry
              index={index}
              isLatest={index === 0}
              key={version.id}
              previousConfig={previousVersion?.config_snapshot ?? emptyConfig()}
              version={version}
            />
          );
        })}
      </div>

      {versions.length >= 2 && <VersionCompare versions={sorted} />}
    </div>
  );
}
