import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@strait/ui/components/card";
import { ScrollArea } from "@strait/ui/components/scroll-area";
import { cn } from "@strait/ui/utils/index";

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
  job_started: "bg-info",
  job_completed: "bg-success",
  job_failed: "bg-destructive",
  workflow_completed: "bg-success",
  approval_pending: "bg-warning",
};

const EVENTS: ActivityEvent[] = [
  {
    id: "1",
    type: "job_completed",
    message: "payment-sync completed in 1.2s",
    timestamp: "2 min ago",
  },
  {
    id: "2",
    type: "job_started",
    message: "email-dispatch started (run_d4e5)",
    timestamp: "3 min ago",
  },
  {
    id: "3",
    type: "job_failed",
    message: "report-gen failed: timeout exceeded",
    timestamp: "5 min ago",
  },
  {
    id: "4",
    type: "workflow_completed",
    message: "onboarding-flow completed",
    timestamp: "6 min ago",
  },
  {
    id: "5",
    type: "approval_pending",
    message: "deploy-prod awaiting approval",
    timestamp: "8 min ago",
  },
  {
    id: "6",
    type: "job_started",
    message: "cache-warm started (run_q19m)",
    timestamp: "10 min ago",
  },
  {
    id: "7",
    type: "job_completed",
    message: "user-import completed in 4.8s",
    timestamp: "12 min ago",
  },
  {
    id: "8",
    type: "job_failed",
    message: "webhook-relay failed: connection refused",
    timestamp: "14 min ago",
  },
  {
    id: "9",
    type: "job_completed",
    message: "invoice-gen completed in 0.9s",
    timestamp: "15 min ago",
  },
  {
    id: "10",
    type: "job_started",
    message: "data-pipeline started (run_m8k2)",
    timestamp: "17 min ago",
  },
  {
    id: "11",
    type: "workflow_completed",
    message: "signup-verification completed",
    timestamp: "19 min ago",
  },
  {
    id: "12",
    type: "job_completed",
    message: "metrics-agg completed in 2.3s",
    timestamp: "21 min ago",
  },
  {
    id: "13",
    type: "job_failed",
    message: "pdf-export failed: out of memory",
    timestamp: "23 min ago",
  },
  {
    id: "14",
    type: "job_started",
    message: "cache-invalidate started (run_p3x9)",
    timestamp: "25 min ago",
  },
  {
    id: "15",
    type: "job_completed",
    message: "notification-batch completed in 0.4s",
    timestamp: "27 min ago",
  },
  {
    id: "16",
    type: "approval_pending",
    message: "db-migration awaiting approval",
    timestamp: "30 min ago",
  },
  {
    id: "17",
    type: "job_completed",
    message: "search-reindex completed in 12.1s",
    timestamp: "33 min ago",
  },
  {
    id: "18",
    type: "job_started",
    message: "log-rotation started (run_v7w1)",
    timestamp: "35 min ago",
  },
  {
    id: "19",
    type: "workflow_completed",
    message: "billing-cycle completed",
    timestamp: "38 min ago",
  },
  {
    id: "20",
    type: "job_completed",
    message: "health-check completed in 0.1s",
    timestamp: "40 min ago",
  },
];

export function LiveActivityFeed() {
  return (
    <Card>
      <CardHeader className="pb-2">
        <CardTitle className="font-medium text-sm">Live Activity</CardTitle>
      </CardHeader>
      <CardContent className="p-0">
        <ScrollArea className="h-[320px] px-6 pb-6">
          <div className="space-y-3">
            {EVENTS.map((event) => (
              <div className="flex items-start gap-2.5" key={event.id}>
                <span
                  className={cn(
                    "mt-1.5 size-2 shrink-0 rounded-full",
                    DOT_COLORS[event.type]
                  )}
                />
                <div className="min-w-0 flex-1">
                  <p className="text-sm leading-tight">{event.message}</p>
                  <p className="text-muted-foreground text-xs">
                    {event.timestamp}
                  </p>
                </div>
              </div>
            ))}
          </div>
        </ScrollArea>
      </CardContent>
    </Card>
  );
}
