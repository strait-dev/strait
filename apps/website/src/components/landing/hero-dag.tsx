"use client";

import { useEffect, useRef, useState } from "react";

type NodeStatus = "queued" | "executing" | "completed" | "failed" | "approval";

type DagNode = {
  id: string;
  label: string;
  x: number;
  y: number;
  status: NodeStatus;
};

type DagEdge = {
  from: string;
  to: string;
};

const NODES: DagNode[] = [
  { id: "validate", label: "Validate\nPayload", x: 60, y: 180, status: "queued" },
  { id: "inventory", label: "Check\nInventory", x: 200, y: 180, status: "queued" },
  { id: "payment", label: "Charge\nPayment", x: 340, y: 120, status: "queued" },
  { id: "approval", label: "Approval\nGate", x: 340, y: 240, status: "queued" },
  { id: "fulfill", label: "Fulfill\nOrder", x: 480, y: 180, status: "queued" },
  { id: "confirm", label: "Send\nConfirm", x: 620, y: 130, status: "queued" },
  { id: "analytics", label: "Update\nAnalytics", x: 620, y: 230, status: "queued" },
];

const EDGES: DagEdge[] = [
  { from: "validate", to: "inventory" },
  { from: "inventory", to: "payment" },
  { from: "inventory", to: "approval" },
  { from: "payment", to: "fulfill" },
  { from: "approval", to: "fulfill" },
  { from: "fulfill", to: "confirm" },
  { from: "fulfill", to: "analytics" },
];

const SEQUENCE: Array<{ id: string; status: NodeStatus; delay: number }> = [
  { id: "validate", status: "executing", delay: 0 },
  { id: "validate", status: "completed", delay: 800 },
  { id: "inventory", status: "executing", delay: 1000 },
  { id: "inventory", status: "completed", delay: 1800 },
  { id: "payment", status: "executing", delay: 2000 },
  { id: "approval", status: "approval", delay: 2000 },
  { id: "payment", status: "failed", delay: 2800 },
  { id: "payment", status: "executing", delay: 3400 },
  { id: "payment", status: "completed", delay: 4200 },
  { id: "approval", status: "completed", delay: 4500 },
  { id: "fulfill", status: "executing", delay: 4800 },
  { id: "fulfill", status: "completed", delay: 5600 },
  { id: "confirm", status: "executing", delay: 5800 },
  { id: "analytics", status: "executing", delay: 5800 },
  { id: "confirm", status: "completed", delay: 6400 },
  { id: "analytics", status: "completed", delay: 6600 },
];

const CYCLE_DURATION = 8000;

const statusColor: Record<NodeStatus, string> = {
  queued: "var(--muted-foreground)",
  executing: "var(--primary)",
  completed: "var(--success)",
  failed: "var(--destructive)",
  approval: "var(--warning)",
};

const statusBg: Record<NodeStatus, string> = {
  queued: "oklch(0.5 0 0 / 0.08)",
  executing: "color-mix(in oklch, var(--primary) 12%, transparent)",
  completed: "color-mix(in oklch, var(--success) 12%, transparent)",
  failed: "color-mix(in oklch, var(--destructive) 12%, transparent)",
  approval: "color-mix(in oklch, var(--warning) 12%, transparent)",
};

const NODE_W = 100;
const NODE_H = 52;

function getNodeCenter(node: DagNode) {
  return { cx: node.x + NODE_W / 2, cy: node.y + NODE_H / 2 };
}

const HeroDag = () => {
  const [nodeStates, setNodeStates] = useState<Record<string, NodeStatus>>(() =>
    Object.fromEntries(NODES.map((n) => [n.id, "queued"]))
  );
  const [hoveredNode, setHoveredNode] = useState<string | null>(null);
  const rafRef = useRef<number>(0);
  const containerRef = useRef<HTMLDivElement>(null);
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
      const elapsed = (timestamp - startTime) % CYCLE_DURATION;

      if (elapsed < 100 && lastApplied !== -1) {
        setNodeStates(Object.fromEntries(NODES.map((n) => [n.id, "queued"])));
        lastApplied = -1;
      }

      for (let i = 0; i < SEQUENCE.length; i++) {
        const step = SEQUENCE[i];
        if (step && elapsed >= step.delay && i > lastApplied) {
          lastApplied = i;
          setNodeStates((prev) => ({ ...prev, [step.id]: step.status }));
        }
      }

      rafRef.current = requestAnimationFrame(tick);
    };

    rafRef.current = requestAnimationFrame(tick);
    return () => cancelAnimationFrame(rafRef.current);
  }, []);

  const nodeMap = Object.fromEntries(NODES.map((n) => [n.id, n]));

  return (
    <div
      ref={containerRef}
      className="relative h-full w-full select-none"
      role="img"
      aria-label="Animated DAG showing an order processing workflow executing"
    >
      <svg
        viewBox="0 0 740 380"
        className="h-full w-full"
        fill="none"
        xmlns="http://www.w3.org/2000/svg"
      >
        {EDGES.map((edge) => {
          const from = nodeMap[edge.from];
          const to = nodeMap[edge.to];
          if (!(from && to)) {
            return null;
          }
          const { cx: x1, cy: y1 } = getNodeCenter(from);
          const { cx: x2, cy: y2 } = getNodeCenter(to);
          const fromStatus = nodeStates[edge.from] ?? "queued";
          const isActive = fromStatus === "completed";

          return (
            <line
              key={`${edge.from}-${edge.to}`}
              x1={x1}
              y1={y1}
              x2={x2}
              y2={y2}
              stroke={isActive ? "var(--success)" : "var(--border)"}
              strokeWidth={1.5}
              strokeDasharray={isActive ? "none" : "4 4"}
              opacity={isActive ? 0.6 : 0.3}
              style={{
                transition: "stroke 0.4s ease, opacity 0.4s ease",
              }}
            />
          );
        })}

        {NODES.map((node) => {
          const status = nodeStates[node.id] ?? "queued";
          const isHovered = hoveredNode === node.id;
          const lines = node.label.split("\n");

          return (
            <g
              key={node.id}
              onPointerEnter={() => setHoveredNode(node.id)}
              onPointerLeave={() => setHoveredNode(null)}
            >
              <rect
                x={node.x}
                y={node.y}
                width={NODE_W}
                height={NODE_H}
                rx={10}
                fill={statusBg[status]}
                stroke={statusColor[status]}
                strokeWidth={isHovered ? 2 : 1.5}
                style={{
                  transition: "fill 0.4s ease, stroke 0.4s ease",
                }}
              />

              {status === "executing" && (
                <rect
                  x={node.x}
                  y={node.y}
                  width={NODE_W}
                  height={NODE_H}
                  rx={10}
                  fill="none"
                  stroke={statusColor[status]}
                  strokeWidth={1}
                  opacity={0.4}
                  style={{
                    animation: "dag-pulse 1.5s ease-in-out infinite",
                  }}
                />
              )}

              {status === "approval" && (
                <rect
                  x={node.x}
                  y={node.y}
                  width={NODE_W}
                  height={NODE_H}
                  rx={10}
                  fill="none"
                  stroke={statusColor[status]}
                  strokeWidth={1}
                  opacity={0.5}
                  style={{
                    animation: "dag-pulse 1s ease-in-out infinite",
                  }}
                />
              )}

              {lines.map((line, i) => (
                <text
                  key={`${node.id}-line-${String(i)}`}
                  x={node.x + NODE_W / 2}
                  y={node.y + NODE_H / 2 + (i - (lines.length - 1) / 2) * 14}
                  textAnchor="middle"
                  dominantBaseline="central"
                  fill={statusColor[status]}
                  fontSize={11}
                  fontFamily="var(--font-sans), system-ui, sans-serif"
                  fontWeight={500}
                  style={{
                    transition: "fill 0.4s ease",
                  }}
                >
                  {line}
                </text>
              ))}

              {isHovered && (
                <g>
                  <rect
                    x={node.x - 10}
                    y={node.y + NODE_H + 8}
                    width={NODE_W + 20}
                    height={42}
                    rx={6}
                    fill="var(--card)"
                    stroke="var(--border)"
                    strokeWidth={1}
                  />
                  <text
                    x={node.x + NODE_W / 2}
                    y={node.y + NODE_H + 24}
                    textAnchor="middle"
                    fill="var(--muted-foreground)"
                    fontSize={9}
                    fontFamily="var(--font-sans), system-ui, sans-serif"
                  >
                    {`Status: ${status}`}
                  </text>
                  <text
                    x={node.x + NODE_W / 2}
                    y={node.y + NODE_H + 38}
                    textAnchor="middle"
                    fill="var(--muted-foreground)"
                    fontSize={9}
                    fontFamily="var(--font-sans), system-ui, sans-serif"
                  >
                    {`run_${node.id.slice(0, 6)} · ${status === "completed" ? "215ms" : "—"}`}
                  </text>
                </g>
              )}
            </g>
          );
        })}
      </svg>

      <style>{`
        @keyframes dag-pulse {
          0%, 100% { opacity: 0.1; transform: scale(1); }
          50% { opacity: 0.5; transform: scale(1.03); }
        }
      `}</style>
    </div>
  );
};

export default HeroDag;
