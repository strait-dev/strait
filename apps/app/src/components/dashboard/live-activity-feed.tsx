import { HugeiconsIcon } from "@hugeicons/react";
import { Button } from "@strait/ui/components/button.tsx";
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@strait/ui/components/card.tsx";
import { cn } from "@strait/ui/utils/index.ts";
import { useCallback, useEffect, useRef, useState } from "react";
import { PauseActionIcon, PlayActionIcon } from "@/lib/icons.ts";

type ActivityEventType =
  | "job_started"
  | "job_completed"
  | "job_failed"
  | "workflow_completed"
  | "approval_pending";

type ActivityEvent = {
  id: string;
  type: ActivityEventType;
  message: string;
  timestamp: string;
};

const DOT_COLORS: Record<ActivityEventType, string> = {
  job_started: "bg-chart-3",
  job_completed: "bg-chart-1",
  job_failed: "bg-chart-4",
  workflow_completed: "bg-chart-1",
  approval_pending: "bg-chart-3",
};

const SAMPLE_MESSAGES: { type: ActivityEventType; message: string }[] = [
  { type: "job_started", message: "payment-sync started (run_x82k)" },
  { type: "job_completed", message: "email-dispatch completed in 1.2s" },
  { type: "job_failed", message: "report-gen failed: timeout exceeded" },
  { type: "workflow_completed", message: "onboarding-flow completed" },
  { type: "approval_pending", message: "deploy-prod awaiting approval" },
  { type: "job_started", message: "cache-warm started (run_q19m)" },
  { type: "job_completed", message: "user-import completed in 4.8s" },
  { type: "job_failed", message: "webhook-relay failed: connection refused" },
];

let nextId = 0;

function makeEvent(): ActivityEvent {
  const sample =
    SAMPLE_MESSAGES[Math.floor(Math.random() * SAMPLE_MESSAGES.length)];
  return {
    id: String(++nextId),
    type: sample.type,
    message: sample.message,
    timestamp: new Date().toLocaleTimeString(),
  };
}

const MAX_EVENTS = 20;

export function LiveActivityFeed() {
  const [events, setEvents] = useState<ActivityEvent[]>(() =>
    Array.from({ length: 5 }, makeEvent)
  );
  const [paused, setPaused] = useState(false);
  const pausedRef = useRef(paused);

  useEffect(() => {
    pausedRef.current = paused;
  }, [paused]);

  const addEvent = useCallback(() => {
    if (pausedRef.current) {
      return;
    }
    setEvents((prev) => [makeEvent(), ...prev].slice(0, MAX_EVENTS));
  }, []);

  useEffect(() => {
    const interval = setInterval(addEvent, 3000);
    return () => clearInterval(interval);
  }, [addEvent]);

  return (
    <Card>
      <CardHeader className="flex flex-row items-center justify-between pb-2">
        <CardTitle className="font-medium text-sm">Live Activity</CardTitle>
        <Button
          className="h-7 w-7"
          onClick={() => setPaused((p) => !p)}
          size="icon"
          variant="ghost"
        >
          <HugeiconsIcon
            icon={paused ? PlayActionIcon : PauseActionIcon}
            size={14}
          />
        </Button>
      </CardHeader>
      <CardContent>
        <div className="max-h-[320px] space-y-3 overflow-y-auto">
          {events.map((event) => (
            <div className="flex items-start gap-2.5" key={event.id}>
              <span
                className={cn(
                  "mt-1.5 h-2 w-2 shrink-0 rounded-full",
                  DOT_COLORS[event.type]
                )}
              />
              <div className="min-w-0 flex-1">
                <p className="text-sm leading-tight">{event.message}</p>
                <p className="text-[11px] text-muted-foreground">
                  {event.timestamp}
                </p>
              </div>
            </div>
          ))}
        </div>
      </CardContent>
    </Card>
  );
}
