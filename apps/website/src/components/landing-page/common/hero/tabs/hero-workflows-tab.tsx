import { useEffect, useRef, useState } from "react";

type NodeStatus = "queued" | "executing" | "completed" | "failed" | "approval";

type PipelineNode = {
  id: string;
  label: string;
};

const NODES: PipelineNode[] = [
  { id: "validate", label: "Validate" },
  { id: "enrich", label: "Enrich" },
  { id: "charge", label: "Charge" },
  { id: "reserve", label: "Reserve" },
  { id: "approval", label: "Approve" },
  { id: "confirm", label: "Confirm" },
];

type LogEntry = {
  time: string;
  text: string;
  type: "info" | "warn" | "error" | "success";
};

const SEQUENCE: Array<{
  delay: number;
  nodeId: string;
  status: NodeStatus;
  log: LogEntry;
}> = [
  {
    delay: 0,
    nodeId: "validate",
    status: "executing",
    log: {
      time: "12:04:01.003",
      text: "validate claimed by worker-2",
      type: "info",
    },
  },
  {
    delay: 700,
    nodeId: "validate",
    status: "completed",
    log: {
      time: "12:04:01.218",
      text: "validate completed (215ms)",
      type: "success",
    },
  },
  {
    delay: 1000,
    nodeId: "enrich",
    status: "executing",
    log: {
      time: "12:04:01.221",
      text: "enrich claimed by worker-1",
      type: "info",
    },
  },
  {
    delay: 1800,
    nodeId: "enrich",
    status: "completed",
    log: {
      time: "12:04:01.845",
      text: "enrich completed (624ms)",
      type: "success",
    },
  },
  {
    delay: 2100,
    nodeId: "charge",
    status: "executing",
    log: {
      time: "12:04:01.850",
      text: "charge claimed by worker-3",
      type: "info",
    },
  },
  {
    delay: 2100,
    nodeId: "reserve",
    status: "executing",
    log: {
      time: "12:04:01.852",
      text: "reserve claimed by worker-2",
      type: "info",
    },
  },
  {
    delay: 3000,
    nodeId: "reserve",
    status: "completed",
    log: {
      time: "12:04:02.710",
      text: "reserve completed (858ms)",
      type: "success",
    },
  },
  {
    delay: 3400,
    nodeId: "charge",
    status: "completed",
    log: {
      time: "12:04:02.920",
      text: "charge completed (1070ms)",
      type: "success",
    },
  },
  {
    delay: 3700,
    nodeId: "approval",
    status: "approval",
    log: {
      time: "12:04:03.925",
      text: "approval gate awaiting review",
      type: "warn",
    },
  },
  {
    delay: 5200,
    nodeId: "approval",
    status: "completed",
    log: {
      time: "12:04:05.430",
      text: "approved by admin@company.io",
      type: "success",
    },
  },
  {
    delay: 5500,
    nodeId: "confirm",
    status: "executing",
    log: {
      time: "12:04:05.435",
      text: "confirm claimed by worker-1",
      type: "info",
    },
  },
  {
    delay: 6300,
    nodeId: "confirm",
    status: "completed",
    log: {
      time: "12:04:06.120",
      text: "confirm completed (685ms)",
      type: "success",
    },
  },
];

const CYCLE_DURATION = 8000;

const statusClass: Record<NodeStatus, string> = {
  queued: "border-border/60 bg-muted/30 text-muted-foreground",
  executing: "border-primary/40 bg-primary/8 text-primary",
  completed: "border-success/40 bg-success/8 text-success",
  failed: "border-destructive/40 bg-destructive/8 text-destructive",
  approval: "border-warning/40 bg-warning/8 text-warning",
};

const logColor: Record<string, string> = {
  info: "text-muted-foreground",
  warn: "text-warning",
  error: "text-destructive",
  success: "text-success",
};

const HeroWorkflowsTab = () => {
  const [nodeStates, setNodeStates] = useState<Record<string, NodeStatus>>(() =>
    Object.fromEntries(NODES.map((n) => [n.id, "queued"]))
  );
  const [logs, setLogs] = useState<LogEntry[]>([]);
  const rafRef = useRef<number>(0);
  const logRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    let startTime: number | null = null;
    let lastApplied = -1;

    const tick = (timestamp: number) => {
      if (startTime === null) {
        startTime = timestamp;
      }
      const elapsed = (timestamp - startTime) % CYCLE_DURATION;

      if (elapsed < 100 && lastApplied !== -1) {
        setNodeStates(Object.fromEntries(NODES.map((n) => [n.id, "queued"])));
        setLogs([]);
        lastApplied = -1;
      }

      for (let i = 0; i < SEQUENCE.length; i++) {
        const step = SEQUENCE[i];
        if (step && elapsed >= step.delay && i > lastApplied) {
          lastApplied = i;
          setNodeStates((prev) => ({ ...prev, [step.nodeId]: step.status }));
          setLogs((prev) => [...prev, step.log]);
        }
      }

      rafRef.current = requestAnimationFrame(tick);
    };

    rafRef.current = requestAnimationFrame(tick);
    return () => cancelAnimationFrame(rafRef.current);
  }, []);

  // biome-ignore lint/correctness/useExhaustiveDependencies: scroll on log change is intentional
  useEffect(() => {
    if (logRef.current) {
      logRef.current.scrollTop = logRef.current.scrollHeight;
    }
  }, [logs]);

  return (
    <div className="flex h-full flex-col">
      {/* DAG visualization */}
      <div className="flex items-center justify-center gap-2 border-border/40 border-b px-4 py-5 sm:gap-3 sm:px-6 sm:py-6">
        {NODES.map((node, idx) => (
          <div className="flex items-center gap-2 sm:gap-3" key={node.id}>
            <div
              className={`rounded-lg border px-2.5 py-1.5 text-center font-medium text-[11px] transition-all duration-400 sm:px-3 sm:py-2 sm:text-xs ${statusClass[nodeStates[node.id] ?? "queued"]}`}
            >
              {node.label}
              {nodeStates[node.id] === "executing" && (
                <span className="ml-1 inline-block size-1.5 animate-pulse rounded-full bg-primary" />
              )}
              {nodeStates[node.id] === "approval" && (
                <span className="ml-1 inline-block size-1.5 animate-pulse rounded-full bg-warning" />
              )}
            </div>
            {idx < NODES.length - 1 && (
              <svg
                className="hidden size-3.5 shrink-0 text-border/60 sm:block"
                fill="none"
                viewBox="0 0 16 16"
              >
                <path
                  d="M3 8h10M10 5l3 3-3 3"
                  stroke="currentColor"
                  strokeLinecap="round"
                  strokeLinejoin="round"
                  strokeWidth={1.5}
                />
              </svg>
            )}
          </div>
        ))}
      </div>

      {/* Event log */}
      <div className="flex min-h-0 flex-1 flex-col bg-muted/20">
        <div className="flex items-center border-border/40 border-b px-4 py-1.5">
          <span className="font-medium text-[11px] text-muted-foreground/60">
            Event Log
          </span>
        </div>
        <div
          className="flex-1 overflow-y-auto px-4 py-2 font-mono text-[11px] leading-relaxed"
          ref={logRef}
        >
          {logs.length === 0 && (
            <p className="text-muted-foreground/30">Waiting for workflow...</p>
          )}
          {logs.map((log, i) => (
            <div
              className="flex animate-fade-in-up gap-2.5 py-px"
              key={`log-${String(i)}`}
              style={{ animationDuration: "150ms" }}
            >
              <span className="shrink-0 text-muted-foreground/30">
                {log.time}
              </span>
              <span className={logColor[log.type]}>{log.text}</span>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
};

export default HeroWorkflowsTab;
