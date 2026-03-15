"use client";

import { useEffect, useRef, useState } from "react";
import Shell from "@/components/layout/shell.tsx";
import MockBrowserWindow from "@/components/magicui/mock-browser-window.tsx";

type NodeStatus = "queued" | "executing" | "completed" | "failed" | "approval";

type PipelineNode = {
  id: string;
  label: string;
  col: number;
  row: number;
};

const PIPELINE_NODES: PipelineNode[] = [
  { id: "validate", label: "Validate Payload", col: 0, row: 0 },
  { id: "enrich", label: "Enrich Data", col: 1, row: 0 },
  { id: "charge", label: "Charge Payment", col: 2, row: -1 },
  { id: "reserve", label: "Reserve Inventory", col: 2, row: 1 },
  { id: "approval", label: "Approval Gate", col: 3, row: 0 },
  { id: "confirm", label: "Send Confirmation", col: 4, row: 0 },
];

type LogEntry = {
  time: string;
  text: string;
  type: "info" | "warn" | "error" | "success";
};

const DEMO_SEQUENCE: Array<{
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
      text: "validate_payload claimed by worker-2",
      type: "info",
    },
  },
  {
    delay: 800,
    nodeId: "validate",
    status: "completed",
    log: {
      time: "12:04:01.218",
      text: "validate_payload completed (215ms)",
      type: "success",
    },
  },
  {
    delay: 1200,
    nodeId: "enrich",
    status: "executing",
    log: {
      time: "12:04:01.221",
      text: "enrich_data claimed by worker-1",
      type: "info",
    },
  },
  {
    delay: 2200,
    nodeId: "enrich",
    status: "completed",
    log: {
      time: "12:04:01.845",
      text: "enrich_data completed (624ms)",
      type: "success",
    },
  },
  {
    delay: 2600,
    nodeId: "charge",
    status: "executing",
    log: {
      time: "12:04:01.850",
      text: "charge_payment claimed by worker-3",
      type: "info",
    },
  },
  {
    delay: 2600,
    nodeId: "reserve",
    status: "executing",
    log: {
      time: "12:04:01.852",
      text: "reserve_inventory claimed by worker-2",
      type: "info",
    },
  },
  {
    delay: 3400,
    nodeId: "charge",
    status: "failed",
    log: {
      time: "12:04:02.460",
      text: "charge_payment failed: 503 gateway timeout",
      type: "error",
    },
  },
  {
    delay: 3800,
    nodeId: "reserve",
    status: "completed",
    log: {
      time: "12:04:02.710",
      text: "reserve_inventory completed (858ms)",
      type: "success",
    },
  },
  {
    delay: 4200,
    nodeId: "charge",
    status: "executing",
    log: {
      time: "12:04:03.100",
      text: "charge_payment retry attempt 2 (backoff 640ms)",
      type: "warn",
    },
  },
  {
    delay: 5200,
    nodeId: "charge",
    status: "completed",
    log: {
      time: "12:04:03.920",
      text: "charge_payment completed (820ms)",
      type: "success",
    },
  },
  {
    delay: 5600,
    nodeId: "approval",
    status: "approval",
    log: {
      time: "12:04:03.925",
      text: "approval_gate awaiting approval",
      type: "warn",
    },
  },
  {
    delay: 7100,
    nodeId: "approval",
    status: "completed",
    log: {
      time: "12:04:05.430",
      text: "approval_gate approved by admin@company.io",
      type: "success",
    },
  },
  {
    delay: 7500,
    nodeId: "confirm",
    status: "executing",
    log: {
      time: "12:04:05.435",
      text: "send_confirmation claimed by worker-1",
      type: "info",
    },
  },
  {
    delay: 8300,
    nodeId: "confirm",
    status: "completed",
    log: {
      time: "12:04:06.120",
      text: "send_confirmation completed (685ms)",
      type: "success",
    },
  },
];

const CYCLE_DURATION = 10_000;

const statusColors: Record<NodeStatus, string> = {
  queued: "border-border/60 bg-muted/30 text-muted-foreground",
  executing: "border-primary/40 bg-primary/8 text-primary",
  completed: "border-success/40 bg-success/8 text-success",
  failed: "border-destructive/40 bg-destructive/8 text-destructive",
  approval: "border-warning/40 bg-warning/8 text-warning",
};

const logTypeColors: Record<string, string> = {
  info: "text-muted-foreground",
  warn: "text-warning",
  error: "text-destructive",
  success: "text-success",
};

const PipelineDemo = () => {
  const [nodeStates, setNodeStates] = useState<Record<string, NodeStatus>>(() =>
    Object.fromEntries(PIPELINE_NODES.map((n) => [n.id, "queued"]))
  );
  const [logs, setLogs] = useState<LogEntry[]>([]);
  const [speed, setSpeed] = useState(1);
  const rafRef = useRef<number>(0);
  const containerRef = useRef<HTMLDivElement>(null);
  const logRef = useRef<HTMLDivElement>(null);
  const isVisibleRef = useRef(true);

  useEffect(() => {
    const el = containerRef.current;
    if (!el) {
      return;
    }
    const obs = new IntersectionObserver(
      ([entry]) => {
        isVisibleRef.current = !!entry?.isIntersecting;
      },
      { threshold: 0.1 }
    );
    obs.observe(el);
    return () => obs.disconnect();
  }, []);

  useEffect(() => {
    let startTime: number | null = null;
    let lastApplied = -1;

    const tick = (timestamp: number) => {
      if (!isVisibleRef.current) {
        rafRef.current = requestAnimationFrame(tick);
        return;
      }

      if (startTime === null) {
        startTime = timestamp;
      }
      const elapsed = ((timestamp - startTime) * speed) % CYCLE_DURATION;

      if (elapsed < 100 && lastApplied !== -1) {
        setNodeStates(
          Object.fromEntries(PIPELINE_NODES.map((n) => [n.id, "queued"]))
        );
        setLogs([]);
        lastApplied = -1;
      }

      for (let i = 0; i < DEMO_SEQUENCE.length; i++) {
        const step = DEMO_SEQUENCE[i];
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
  }, [speed]);

  useEffect(() => {
    if (logRef.current) {
      logRef.current.scrollTop = logRef.current.scrollHeight;
    }
  }, []);

  const resetDemo = () => {
    setNodeStates(
      Object.fromEntries(PIPELINE_NODES.map((n) => [n.id, "queued"]))
    );
    setLogs([]);
  };

  return (
    <section className="py-20 sm:py-28" ref={containerRef}>
      <Shell variant="wide">
        <div className="mb-14 max-w-3xl">
          <h2 className="text-balance text-2xl leading-[1.2] tracking-tight sm:text-3xl lg:text-4xl">
            <span className="text-foreground">
              Watch a workflow execute in real time.
            </span>{" "}
            <span className="text-muted-foreground">
              From trigger to completion — retries, approval gates, and fan-out
              in a single replay.
            </span>
          </h2>
        </div>

        <MockBrowserWindow
          actions={
            <div className="flex items-center gap-2">
              {[1, 2, 4].map((s) => (
                <button
                  className={`rounded px-1.5 py-0.5 text-xs transition-colors ${
                    speed === s
                      ? "bg-foreground/10 text-foreground"
                      : "text-muted-foreground/50 hover:text-muted-foreground"
                  }`}
                  key={s}
                  onClick={() => setSpeed(s)}
                  type="button"
                >
                  {s}x
                </button>
              ))}
            </div>
          }
          className="shadow-lg"
          url="strait — workflow replay"
        >
          {/* DAG area */}
          <div className="border-border/50 border-b p-6 sm:p-8">
            <div className="flex flex-wrap items-center justify-center gap-3 sm:gap-4">
              {PIPELINE_NODES.map((node, idx) => {
                const status = nodeStates[node.id] ?? "queued";
                return (
                  <div
                    className="flex items-center gap-3 sm:gap-4"
                    key={node.id}
                  >
                    <div
                      className={`rounded-lg border px-3 py-2 text-center font-medium text-xs transition-all duration-400 sm:px-4 sm:py-2.5 sm:text-sm ${statusColors[status]}`}
                    >
                      {node.label}
                      {status === "approval" && (
                        <span className="ml-1.5 inline-block size-1.5 animate-pulse rounded-full bg-warning" />
                      )}
                      {status === "executing" && (
                        <span className="ml-1.5 inline-block size-1.5 animate-pulse rounded-full bg-primary" />
                      )}
                    </div>
                    {idx < PIPELINE_NODES.length - 1 && (
                      <svg
                        className="hidden size-4 text-border sm:block"
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
                );
              })}
            </div>
          </div>

          {/* Event log */}
          <div className="bg-muted/30">
            <div className="flex items-center justify-between border-border/50 border-b px-4 py-2">
              <span className="font-medium text-muted-foreground text-xs">
                Event Log
              </span>
              <button
                className="text-muted-foreground/60 text-xs transition-colors hover:text-foreground"
                onClick={resetDemo}
                type="button"
              >
                Replay
              </button>
            </div>
            <div
              className="h-48 overflow-y-auto p-4 font-mono text-xs"
              ref={logRef}
            >
              {logs.length === 0 && (
                <p className="text-muted-foreground/40">
                  Waiting for workflow to start...
                </p>
              )}
              {logs.map((log, i) => (
                <div
                  className="flex animate-fade-in-up gap-3 py-0.5"
                  key={`log-${String(i)}`}
                  style={{ animationDuration: "200ms" }}
                >
                  <span className="shrink-0 text-muted-foreground/40">
                    {log.time}
                  </span>
                  <span className={logTypeColors[log.type]}>{log.text}</span>
                </div>
              ))}
            </div>
          </div>
        </MockBrowserWindow>

        {/* Stat cards */}
        <div className="mt-8 grid grid-cols-1 gap-4 sm:grid-cols-3">
          <div className="rounded-xl border border-border/60 bg-card p-4 sm:p-5">
            <p className="font-heading font-semibold text-2xl text-foreground">
              Full lifecycle tracking
            </p>
            <p className="mt-1 text-muted-foreground text-sm">
              Every run state visible from queued to completed
            </p>
          </div>
          <div className="rounded-xl border border-border/60 bg-card p-4 sm:p-5">
            <p className="font-heading font-semibold text-2xl text-foreground">
              Real-time streaming
            </p>
            <p className="mt-1 text-muted-foreground text-sm">
              Stream state changes instantly to your webhooks
            </p>
          </div>
          <div className="rounded-xl border border-border/60 bg-card p-4 sm:p-5">
            <p className="font-heading font-semibold text-2xl text-foreground">
              Debug bundles
            </p>
            <p className="mt-1 text-muted-foreground text-sm">
              Inspect any failed run with full execution context
            </p>
          </div>
        </div>
      </Shell>
    </section>
  );
};

export default PipelineDemo;
