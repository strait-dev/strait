import type { ExecutionTrace } from "@/hooks/api/types";

export const TRACE_SEGMENTS: {
  key: keyof ExecutionTrace;
  label: string;
  color: string;
}[] = [
  { key: "queue_wait_ms", label: "Queue Wait", color: "bg-blue-500" },
  { key: "dequeue_ms", label: "Dequeue", color: "bg-indigo-500" },
  { key: "dispatch_ms", label: "Dispatch", color: "bg-violet-500" },
  { key: "connect_ms", label: "Connect", color: "bg-amber-500" },
  { key: "ttfb_ms", label: "TTFB", color: "bg-emerald-500" },
  { key: "transfer_ms", label: "Transfer", color: "bg-cyan-500" },
];

const ExecutionTraceBar = ({ trace }: { trace: ExecutionTrace }) => {
  const total = trace.total_ms || 1;

  return (
    <div className="space-y-3">
      {/* Bar visualization */}
      <div className="flex h-6 w-full overflow-hidden rounded-md">
        {TRACE_SEGMENTS.map((seg) => {
          const ms = trace[seg.key];
          const pct = (ms / total) * 100;
          if (pct < 0.5) {
            return null;
          }
          return (
            <div
              className={`${seg.color} opacity-80`}
              key={seg.key}
              style={{ width: `${pct}%` }}
              title={`${seg.label}: ${ms}ms`}
            />
          );
        })}
      </div>

      {/* Legend / key-value list */}
      <div className="grid grid-cols-2 gap-x-6 gap-y-2 sm:grid-cols-3">
        {TRACE_SEGMENTS.map((seg) => (
          <div className="flex items-center gap-2" key={seg.key}>
            <span
              className={`inline-block h-2.5 w-2.5 rounded-sm ${seg.color} opacity-80`}
            />
            <span className="text-muted-foreground text-xs">{seg.label}</span>
            <span className="font-mono text-xs">{trace[seg.key]}ms</span>
          </div>
        ))}
        <div className="flex items-center gap-2">
          <span className="inline-block h-2.5 w-2.5 rounded-sm bg-foreground/20" />
          <span className="text-muted-foreground text-xs">Total</span>
          <span className="font-mono text-xs">{trace.total_ms}ms</span>
        </div>
      </div>
    </div>
  );
};

export default ExecutionTraceBar;
