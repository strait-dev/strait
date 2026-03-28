
import { useEffect, useRef, useState } from "react";

const HeroObservabilityTab = () => {
  const [score, setScore] = useState(0);
  const [queueDepth, setQueueDepth] = useState(0);
  const [throughput, setThroughput] = useState(0);
  const [latencyP99, setLatencyP99] = useState(0);
  const rafRef = useRef<number>(0);
  const [recentRuns, setRecentRuns] = useState<
    Array<{
      id: string;
      job: string;
      status: string;
      duration: string;
      statusClass: string;
    }>
  >([]);

  useEffect(() => {
    let startTime: number | null = null;

    const tick = (timestamp: number) => {
      if (startTime === null) {
        startTime = timestamp;
      }
      const elapsed = (timestamp - startTime) % 6000;
      const progress = elapsed / 6000;

      setScore(Math.min(Math.round(progress * 96), 96));
      setQueueDepth(Math.round(Math.sin(progress * Math.PI * 2) * 8 + 12));
      setThroughput(Math.round(progress * 1240 + Math.sin(progress * 10) * 60));
      setLatencyP99(Math.round(45 + Math.sin(progress * Math.PI * 3) * 15));

      const runCount = Math.min(Math.floor(progress * 6), 5);
      const allRuns = [
        {
          id: "run_a3f9",
          job: "process-order",
          status: "completed",
          duration: "215ms",
          statusClass: "text-success",
        },
        {
          id: "run_b71e",
          job: "send-receipt",
          status: "completed",
          duration: "89ms",
          statusClass: "text-success",
        },
        {
          id: "run_c92d",
          job: "sync-inventory",
          status: "executing",
          duration: "—",
          statusClass: "text-primary",
        },
        {
          id: "run_d18f",
          job: "charge-payment",
          status: "failed",
          duration: "1.2s",
          statusClass: "text-destructive",
        },
        {
          id: "run_e45a",
          job: "enrich-data",
          status: "completed",
          duration: "624ms",
          statusClass: "text-success",
        },
      ];
      setRecentRuns(allRuns.slice(0, runCount));

      rafRef.current = requestAnimationFrame(tick);
    };

    rafRef.current = requestAnimationFrame(tick);
    return () => cancelAnimationFrame(rafRef.current);
  }, []);

  const bars = [
    { label: "Queue", value: Math.min(score * 1.05, 92) },
    { label: "Workers", value: Math.min(score * 0.98, 88) },
    { label: "Latency", value: Math.min(score * 1.02, 96) },
  ];

  return (
    <div className="flex h-full flex-col">
      {/* Metrics row */}
      <div className="grid grid-cols-4 gap-px border-border/40 border-b bg-border/40">
        <div className="bg-card px-4 py-4 text-center">
          <p className="font-mono text-[10px] text-muted-foreground/50">
            HEALTH
          </p>
          <p className="mt-1 font-semibold text-2xl text-foreground tabular-nums">
            {score}
          </p>
          <p className="text-[10px] text-muted-foreground/40">/100</p>
        </div>
        <div className="bg-card px-4 py-4 text-center">
          <p className="font-mono text-[10px] text-muted-foreground/50">
            QUEUE
          </p>
          <p className="mt-1 font-semibold text-2xl text-foreground tabular-nums">
            {queueDepth}
          </p>
          <p className="text-[10px] text-muted-foreground/40">pending</p>
        </div>
        <div className="bg-card px-4 py-4 text-center">
          <p className="font-mono text-[10px] text-muted-foreground/50">
            THROUGHPUT
          </p>
          <p className="mt-1 font-semibold text-2xl text-foreground tabular-nums">
            {throughput}
          </p>
          <p className="text-[10px] text-muted-foreground/40">runs/hr</p>
        </div>
        <div className="bg-card px-4 py-4 text-center">
          <p className="font-mono text-[10px] text-muted-foreground/50">
            P99 LATENCY
          </p>
          <p className="mt-1 font-semibold text-2xl text-foreground tabular-nums">
            {latencyP99}
          </p>
          <p className="text-[10px] text-muted-foreground/40">ms</p>
        </div>
      </div>

      {/* Health bars + recent runs */}
      <div className="flex min-h-0 flex-1">
        {/* Health breakdown */}
        <div className="w-1/3 border-border/40 border-r px-4 py-4">
          <p className="mb-3 font-medium text-[10px] text-muted-foreground/50 uppercase tracking-wider">
            Health
          </p>
          <div className="space-y-3">
            {bars.map((bar) => (
              <div key={bar.label}>
                <div className="mb-1 flex items-center justify-between">
                  <span className="text-[11px] text-muted-foreground/60">
                    {bar.label}
                  </span>
                  <span className="font-mono text-[10px] text-muted-foreground/40 tabular-nums">
                    {Math.round(bar.value)}%
                  </span>
                </div>
                <div className="h-1.5 overflow-hidden rounded-full bg-muted/40">
                  <div
                    className="h-full rounded-full bg-primary/50 transition-transform duration-200"
                    style={{
                      transform: `scaleX(${bar.value / 100})`,
                      transformOrigin: "left",
                    }}
                  />
                </div>
              </div>
            ))}
          </div>
        </div>

        {/* Recent runs */}
        <div className="flex-1 px-4 py-4">
          <p className="mb-3 font-medium text-[10px] text-muted-foreground/50 uppercase tracking-wider">
            Recent Runs
          </p>
          <div className="space-y-1.5 font-mono text-[11px]">
            {recentRuns.map((run) => (
              <div
                className="flex animate-fade-in-up items-center gap-3 rounded px-2 py-1 transition-colors hover:bg-muted/30"
                key={run.id}
                style={{ animationDuration: "150ms" }}
              >
                <span className="text-muted-foreground/40">{run.id}</span>
                <span className="text-foreground/70">{run.job}</span>
                <span className={`ml-auto ${run.statusClass}`}>
                  {run.status}
                </span>
                <span className="w-12 text-right text-muted-foreground/40">
                  {run.duration}
                </span>
              </div>
            ))}
          </div>
        </div>
      </div>
    </div>
  );
};

export default HeroObservabilityTab;
