import { useEffect, useRef, useState } from "react";

type RunState =
  | "executing"
  | "failed"
  | "retrying"
  | "dead"
  | "completed"
  | "replayed";

type RetryEvent = {
  delay: number;
  state: RunState;
  attempt: number;
  backoff?: string;
  log: {
    time: string;
    text: string;
    type: "info" | "warn" | "error" | "success";
  };
};

const EVENTS: RetryEvent[] = [
  {
    delay: 0,
    state: "executing",
    attempt: 1,
    log: {
      time: "14:22:01.003",
      text: "process_payment claimed by worker-1",
      type: "info",
    },
  },
  {
    delay: 800,
    state: "failed",
    attempt: 1,
    log: {
      time: "14:22:01.840",
      text: "attempt 1 failed: 503 gateway timeout",
      type: "error",
    },
  },
  {
    delay: 1600,
    state: "retrying",
    attempt: 2,
    backoff: "1s",
    log: {
      time: "14:22:02.841",
      text: "retry attempt 2 (backoff 1s)",
      type: "warn",
    },
  },
  {
    delay: 2200,
    state: "failed",
    attempt: 2,
    log: {
      time: "14:22:03.440",
      text: "attempt 2 failed: 503 gateway timeout",
      type: "error",
    },
  },
  {
    delay: 3200,
    state: "retrying",
    attempt: 3,
    backoff: "2s",
    log: {
      time: "14:22:05.441",
      text: "retry attempt 3 (backoff 2s)",
      type: "warn",
    },
  },
  {
    delay: 3800,
    state: "failed",
    attempt: 3,
    log: {
      time: "14:22:06.050",
      text: "attempt 3 failed: 503 gateway timeout",
      type: "error",
    },
  },
  {
    delay: 4400,
    state: "dead",
    attempt: 3,
    log: {
      time: "14:22:06.051",
      text: "max retries reached, routed to DLQ",
      type: "error",
    },
  },
  {
    delay: 5800,
    state: "replayed",
    attempt: 4,
    log: {
      time: "14:22:12.100",
      text: "replayed from DLQ by admin@company.io",
      type: "info",
    },
  },
  {
    delay: 6600,
    state: "completed",
    attempt: 4,
    log: {
      time: "14:22:12.920",
      text: "process_payment completed (820ms)",
      type: "success",
    },
  },
];

const CYCLE_DURATION = 8500;

const stateClass: Record<RunState, string> = {
  executing: "border-primary/40 bg-primary/8 text-primary",
  failed: "border-destructive/40 bg-destructive/8 text-destructive",
  retrying: "border-warning/40 bg-warning/8 text-warning",
  dead: "border-destructive/60 bg-destructive/12 text-destructive",
  completed: "border-success/40 bg-success/8 text-success",
  replayed: "border-primary/40 bg-primary/8 text-primary",
};

const stateLabel: Record<RunState, string> = {
  executing: "Executing",
  failed: "Failed",
  retrying: "Retrying",
  dead: "Dead Letter Queue",
  completed: "Completed",
  replayed: "Replaying",
};

const logColor: Record<string, string> = {
  info: "text-muted-foreground",
  warn: "text-warning",
  error: "text-destructive",
  success: "text-success",
};

const HeroRetriesTab = () => {
  const [state, setState] = useState<RunState>("executing");
  const [attempt, setAttempt] = useState(1);
  const [backoff, setBackoff] = useState<string | undefined>(undefined);
  const [logs, setLogs] = useState<RetryEvent["log"][]>([]);
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
        setState("executing");
        setAttempt(1);
        setBackoff(undefined);
        setLogs([]);
        lastApplied = -1;
      }

      for (let i = 0; i < EVENTS.length; i++) {
        const evt = EVENTS[i];
        if (evt && elapsed >= evt.delay && i > lastApplied) {
          lastApplied = i;
          setState(evt.state);
          setAttempt(evt.attempt);
          setBackoff(evt.backoff);
          setLogs((prev) => [...prev, evt.log]);
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
      {/* Status panel */}
      <div className="flex items-center justify-between border-border/40 border-b px-5 py-5 sm:px-6 sm:py-6">
        <div className="flex items-center gap-4">
          <div>
            <p className="font-mono text-[11px] text-muted-foreground/50">
              run_id
            </p>
            <p className="font-mono text-foreground text-xs">run_p4x92k</p>
          </div>
          <div>
            <p className="font-mono text-[11px] text-muted-foreground/50">
              job
            </p>
            <p className="font-mono text-foreground text-xs">process_payment</p>
          </div>
          <div>
            <p className="font-mono text-[11px] text-muted-foreground/50">
              attempt
            </p>
            <p className="font-mono text-foreground text-xs">{attempt}/3</p>
          </div>
          {backoff && (
            <div>
              <p className="font-mono text-[11px] text-muted-foreground/50">
                backoff
              </p>
              <p className="font-mono text-foreground text-xs">{backoff}</p>
            </div>
          )}
        </div>
        <div
          className={`rounded-full border px-3 py-1 font-medium text-xs transition-all duration-300 ${stateClass[state]}`}
        >
          {stateLabel[state]}
          {(state === "executing" ||
            state === "retrying" ||
            state === "replayed") && (
            <span className="ml-1.5 inline-block size-1.5 animate-pulse rounded-full bg-current" />
          )}
        </div>
      </div>

      {/* Retry timeline + log */}
      <div className="flex min-h-0 flex-1 flex-col bg-muted/20">
        <div className="flex items-center border-border/40 border-b px-4 py-1.5">
          <span className="font-medium text-[11px] text-muted-foreground/60">
            Retry Log
          </span>
        </div>
        <div
          className="flex-1 overflow-y-auto px-4 py-2 font-mono text-[11px] leading-relaxed"
          ref={logRef}
        >
          {logs.map((log, i) => (
            <div
              className="flex animate-fade-in-up gap-2.5 py-px"
              key={`rlog-${String(i)}`}
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

export default HeroRetriesTab;
