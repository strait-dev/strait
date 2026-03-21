"use client";

import { useEffect, useRef, useState } from "react";

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

/* -- 1. Job Orchestration -- */
export const QueueAnimation = () => {
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
            className={`flex gap-8 rounded px-2 py-1 transition-colors duration-150 ${
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

/* -- 3. Managed Execution -- */
export const ExecutionAnimation = () => {
  const tick = useLoopCounter(2000);
  const regions = [
    { label: "iad", status: "warm" },
    { label: "lhr", status: "warm" },
    { label: "nrt", status: "cold" },
  ];
  const activeIdx = tick % regions.length;

  return (
    <div className="flex flex-col gap-2 font-mono text-xs">
      <div className="mb-1 text-muted-foreground/50">Regions</div>
      {regions.map((region, i) => {
        const isActive = i === activeIdx;
        return (
          <div
            className={`flex items-center gap-3 rounded px-2 py-1 transition-colors duration-150 ${
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
export const CostAnimation = () => {
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
export const SdkAnimation = () => {
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
export const HealthAnimation = () => {
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
