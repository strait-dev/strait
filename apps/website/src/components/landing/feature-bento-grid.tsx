"use client";

import { useEffect, useRef, useState } from "react";
import Reveal from "@/components/landing/reveal.tsx";
import Shell from "@/components/layout/shell.tsx";

/* -- Shared animation hook -- */
function useLoopCounter(intervalMs: number, paused = false) {
  const [count, setCount] = useState(0);
  useEffect(() => {
    if (paused) {
      return;
    }
    const id = setInterval(() => setCount((c) => c + 1), intervalMs);
    return () => clearInterval(id);
  }, [intervalMs, paused]);
  return count;
}

/* -- 1. Job Orchestration (adapted from QueueAnimation) -- */
const QueueAnimation = () => {
  const tick = useLoopCounter(1800);
  const rows = [
    { id: "run_a3f", status: "executing", state: "3/13" },
    { id: "run_b71", status: "queued", state: "1/13" },
    { id: "run_c92", status: "retrying", state: "5/13" },
    { id: "run_d18", status: "completed", state: "13/13" },
  ];
  const activeIdx = tick % rows.length;

  return (
    <div className="flex flex-col gap-1.5 font-mono text-xs">
      <div className="mb-1 flex gap-12 text-muted-foreground/50">
        <span>run_id</span>
        <span>status</span>
        <span>lifecycle</span>
      </div>
      {rows.map((row, i) => {
        const isActive = i === activeIdx;
        return (
          <div
            className={`flex gap-8 rounded px-2 py-1 transition-colors duration-500 ${
              isActive
                ? "bg-primary/10 text-primary"
                : "text-muted-foreground/60"
            }`}
            key={row.id}
          >
            <span className="w-16">{row.id}</span>
            <span className="w-20">{isActive ? "claimed" : row.status}</span>
            <span>{row.state}</span>
          </div>
        );
      })}
    </div>
  );
};

/* -- 2. DAG Workflows -- */
const DagAnimation = () => {
  const nodes = [
    { x: 20, y: 30, label: "A" },
    { x: 80, y: 15, label: "B" },
    { x: 80, y: 45, label: "C" },
    { x: 140, y: 30, label: "D" },
    { x: 200, y: 30, label: "E" },
  ];
  const edges: [number, number][] = [
    [0, 1],
    [0, 2],
    [1, 3],
    [2, 3],
    [3, 4],
  ];
  const [hoveredNode, setHoveredNode] = useState<number | null>(null);

  return (
    <svg className="h-full w-full" viewBox="0 0 240 60">
      {edges.map(([from, to]) => {
        const f = nodes[from];
        const t = nodes[to];
        if (!(f && t)) {
          return null;
        }
        return (
          <line
            key={`${String(from)}-${String(to)}`}
            opacity={0.3}
            stroke="var(--primary)"
            strokeWidth={1}
            x1={f.x}
            x2={t.x}
            y1={f.y}
            y2={t.y}
          />
        );
      })}
      {nodes.map((node, i) => (
        <g
          key={node.label}
          onPointerEnter={() => setHoveredNode(i)}
          onPointerLeave={() => setHoveredNode(null)}
        >
          <circle
            cx={node.x}
            cy={node.y}
            fill={
              hoveredNode === i
                ? "color-mix(in oklch, var(--primary) 20%, transparent)"
                : "color-mix(in oklch, var(--primary) 8%, transparent)"
            }
            r={hoveredNode === i ? 12 : 10}
            stroke="var(--primary)"
            strokeWidth={hoveredNode === i ? 1.5 : 1}
            style={{ transition: "all 0.3s ease" }}
          />
          <text
            dominantBaseline="central"
            fill="var(--primary)"
            fontSize={9}
            fontWeight={600}
            textAnchor="middle"
            x={node.x}
            y={node.y}
          >
            {node.label}
          </text>
        </g>
      ))}
    </svg>
  );
};

/* -- 3. Managed Execution -- */
const ExecutionAnimation = () => {
  const tick = useLoopCounter(2000);
  const regions = [
    { label: "iad", status: "warm" },
    { label: "lhr", status: "warm" },
    { label: "nrt", status: "cold" },
  ];
  const activeIdx = tick % regions.length;

  return (
    <div className="flex flex-col gap-2 font-mono text-xs">
      <div className="mb-1 text-muted-foreground/50">Fly Machines</div>
      {regions.map((region, i) => {
        const isActive = i === activeIdx;
        return (
          <div
            className={`flex items-center gap-3 rounded px-2 py-1 transition-colors duration-500 ${
              isActive
                ? "bg-primary/10 text-primary"
                : "text-muted-foreground/60"
            }`}
            key={region.label}
          >
            <div
              className={`size-1.5 rounded-full ${
                isActive ? "bg-success" : "bg-muted-foreground/30"
              }`}
            />
            <span className="w-8">{region.label}</span>
            <span>{isActive ? "executing" : region.status}</span>
          </div>
        );
      })}
    </div>
  );
};

/* -- 4. AI Agent Platform -- */
const CostAnimation = () => {
  const [progress, setProgress] = useState(0);
  useEffect(() => {
    const id = setInterval(() => {
      setProgress((p) => (p >= 78 ? 0 : p + 1));
    }, 40);
    return () => clearInterval(id);
  }, []);

  const circumference = 2 * Math.PI * 40;
  const offset = circumference - (progress / 100) * circumference;

  return (
    <div className="flex flex-col items-center gap-2">
      <svg className="size-24" viewBox="0 0 100 100">
        <circle
          cx="50"
          cy="50"
          fill="none"
          r="40"
          stroke="var(--border)"
          strokeWidth="6"
        />
        <circle
          cx="50"
          cy="50"
          fill="none"
          r="40"
          stroke="var(--primary)"
          strokeDasharray={circumference}
          strokeDashoffset={offset}
          strokeLinecap="round"
          strokeWidth="6"
          style={{
            transform: "rotate(-90deg)",
            transformOrigin: "center",
            transition: "stroke-dashoffset 0.1s ease",
          }}
        />
        <text
          dominantBaseline="central"
          fill="var(--foreground)"
          fontSize="16"
          fontWeight="600"
          textAnchor="middle"
          x="50"
          y="48"
        >
          ${(progress * 0.12).toFixed(0)}
        </text>
        <text
          dominantBaseline="central"
          fill="var(--muted-foreground)"
          fontSize="8"
          textAnchor="middle"
          x="50"
          y="62"
        >
          of $12 budget
        </text>
      </svg>
    </div>
  );
};

/* -- 5. SDK Languages -- */
const SdkAnimation = () => {
  const code = `await strait.runs.create({
  jobId: "process-order",
  workflowId: "checkout-flow",
  payload: orderData,
})`;
  const [chars, setChars] = useState(0);
  const intervalRef = useRef<ReturnType<typeof setInterval>>(undefined);

  useEffect(() => {
    intervalRef.current = setInterval(() => {
      setChars((c) => {
        if (c >= code.length) {
          clearInterval(intervalRef.current);
          return c;
        }
        return c + 1;
      });
    }, 25);
    return () => clearInterval(intervalRef.current);
  }, []);

  return (
    <pre className="animate-gradient-shimmer overflow-hidden rounded-lg bg-[linear-gradient(90deg,_transparent,_var(--primary)/0.03,_transparent)] font-mono text-primary/80 text-xs leading-relaxed">
      <code>{code.slice(0, chars)}</code>
      {chars < code.length && (
        <span className="inline-block h-4 w-0.5 animate-pulse bg-primary/60" />
      )}
    </pre>
  );
};

/* -- 6. Built-in Observability -- */
const HealthAnimation = () => {
  const [score, setScore] = useState(0);
  useEffect(() => {
    const id = setInterval(() => {
      setScore((s) => (s >= 94 ? 0 : s + 1));
    }, 30);
    return () => clearInterval(id);
  }, []);

  const bars = [
    { label: "Queue", value: Math.min(score * 1.1, 92) },
    { label: "Workers", value: Math.min(score * 1.05, 88) },
    { label: "Latency", value: Math.min(score * 0.95, 96) },
  ];

  return (
    <div className="flex flex-col gap-3">
      {bars.map((bar) => (
        <div className="flex items-center gap-3" key={bar.label}>
          <span className="w-14 text-muted-foreground/60 text-xs">
            {bar.label}
          </span>
          <div className="h-2 flex-1 overflow-hidden rounded-full bg-muted/50">
            <div
              className="h-full w-full origin-left rounded-full bg-primary/60 transition-transform duration-100"
              style={{ transform: `scaleX(${bar.value / 100})` }}
            />
          </div>
        </div>
      ))}
      <div className="mt-1 text-center">
        <span className="font-semibold text-2xl text-foreground tabular-nums">
          {score}
        </span>
        <span className="ml-1 text-muted-foreground text-xs">/100</span>
      </div>
    </div>
  );
};

/* -- Feature card data -- */
type FeatureCard = {
  id: string;
  title: string;
  subtitle: string;
  description: string;
  span: string;
  Animation: React.FC;
};

const FEATURES: FeatureCard[] = [
  {
    id: "job-orchestration",
    title: "Job Orchestration",
    subtitle: "Define, trigger, retry, schedule",
    description:
      "Define, trigger, retry, schedule. 13-state run lifecycle with exponential backoff, dead-letter routing, and automatic cleanup.",
    span: "sm:col-span-1 lg:col-span-6",
    Animation: QueueAnimation,
  },
  {
    id: "workflow-dags",
    title: "Workflow DAGs",
    subtitle: "Wire any dependency graph",
    description:
      "Visual pipelines with conditions, approval gates, fan-out/fan-in. Model complex pipelines as directed acyclic graphs.",
    span: "sm:col-span-1 lg:col-span-6",
    Animation: DagAnimation,
  },
  {
    id: "managed-execution",
    title: "Managed Execution",
    subtitle: "Containers on Fly Machines",
    description:
      "Your code runs in containers on Fly Machines. Warm pools, multi-region deployment, and automatic scaling.",
    span: "sm:col-span-1 lg:col-span-6",
    Animation: ExecutionAnimation,
  },
  {
    id: "ai-agent-platform",
    title: "AI Agent Platform",
    subtitle: "Cost tracking and guardrails",
    description:
      "Cost tracking, guardrails, persistent memory, MCP server. Set per-run budgets and track AI model token usage in real time.",
    span: "sm:col-span-1 lg:col-span-6",
    Animation: CostAnimation,
  },
  {
    id: "language-sdks",
    title: "5 Language SDKs",
    subtitle: "TypeScript, Python, Go, Ruby, Rust",
    description:
      "TypeScript, Python, Go, Ruby, Rust. 186 operations each. Logging, heartbeats, checkpoints, and continuation.",
    span: "sm:col-span-1 lg:col-span-6",
    Animation: SdkAnimation,
  },
  {
    id: "observability",
    title: "Built-in Observability",
    subtitle: "OpenTelemetry and health scores",
    description:
      "OpenTelemetry, health scores, cost analytics, structured logs. Composite health scores from queue depth, throughput, and latency.",
    span: "sm:col-span-1 lg:col-span-6",
    Animation: HealthAnimation,
  },
];

/* -- Main component -- */
const FeatureBentoGrid = () => (
  <section className="py-20 sm:py-28" id="features">
    <Shell variant="wide">
      <div className="mb-14 max-w-3xl">
        <h2 className="text-balance text-2xl leading-[1.2] sm:text-3xl lg:text-4xl">
          <span className="text-foreground">
            Everything you need to ship reliable background execution.
          </span>
        </h2>
      </div>

      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-12">
        {FEATURES.map((feature, idx) => (
          <Reveal
            className={`group overflow-hidden rounded-2xl border border-border/40 bg-card/50 shadow-sm transition-shadow duration-300 hover:shadow-md ${feature.span}`}
            delay={idx * 0.06}
            key={feature.id}
            spring
            variant={idx % 2 === 0 ? "fade-up" : "scale"}
          >
            {/* Animation area */}
            <div className="relative flex h-48 items-center justify-center overflow-hidden bg-muted/20 p-6">
              <div className="showcase-dots pointer-events-none absolute inset-0 opacity-30" />
              <div className="relative z-10">
                <feature.Animation />
              </div>
            </div>

            {/* Content */}
            <div className="p-5 sm:p-6">
              <p className="text-muted-foreground text-xs uppercase">
                {feature.subtitle}
              </p>
              <h3 className="mt-1 font-semibold text-foreground text-lg">
                {feature.title}
              </h3>
              <p className="mt-2 text-muted-foreground text-sm leading-relaxed">
                {feature.description}
              </p>
            </div>
          </Reveal>
        ))}
      </div>
    </Shell>
  </section>
);

export default FeatureBentoGrid;
