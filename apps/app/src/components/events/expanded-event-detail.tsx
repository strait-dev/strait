import { Badge } from "@strait/ui/components/badge";
import { Button } from "@strait/ui/components/button";
import { cn } from "@strait/ui/utils/index";

import type { EventTrigger } from "@/hooks/api/types";
import { EVENT_STATUS_STYLES } from "@/lib/status";

const ExpandedEventDetail = ({
  event,
  onClose,
}: {
  event: EventTrigger | null;
  onClose: () => void;
}) => {
  if (!event) {
    return null;
  }

  const style =
    EVENT_STATUS_STYLES[event.status] ?? EVENT_STATUS_STYLES.waiting;

  return (
    <div className="rounded-lg border bg-card p-4">
      <div className="mb-3 flex items-center justify-between">
        <div className="flex items-center gap-2">
          <span className={cn("size-2 shrink-0 rounded-full", style.dot)} />
          <Badge className={cn("capitalize", style.badge)} variant="outline">
            {event.status}
          </Badge>
          <span className="text-muted-foreground text-xs">
            {new Date(event.requested_at).toLocaleString()}
          </span>
        </div>
        <Button onClick={onClose} variant="ghost">
          Close
        </Button>
      </div>
      <p className="mb-2 font-mono text-sm">{event.event_key}</p>
      <div className="flex items-center gap-4 text-muted-foreground text-xs">
        <span>
          Source: <code className="font-mono">{event.source_type}</code>
        </span>
        <span>
          Type: <code className="font-mono">{event.trigger_type}</code>
        </span>
        {event.job_run_id && (
          <span>
            Run: <code className="font-mono">{event.job_run_id}</code>
          </span>
        )}
      </div>
      {event.request_payload != null && (
        <pre className="mt-3 overflow-x-auto rounded-md bg-muted p-3 font-mono text-xs">
          {JSON.stringify(event.request_payload, null, 2)}
        </pre>
      )}
      {event.error && (
        <p className="mt-2 text-destructive text-sm">{event.error}</p>
      )}
    </div>
  );
};

export default ExpandedEventDetail;
