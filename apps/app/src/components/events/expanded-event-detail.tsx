import { Alert, AlertDescription } from "@strait/ui/components/alert";
import { Badge } from "@strait/ui/components/badge";
import { Button } from "@strait/ui/components/button";
import { Card, CardContent } from "@strait/ui/components/card";
import { CodeBlock } from "@strait/ui/components/code-block";
import { StatusBadge } from "@strait/ui/components/status-badge";

import type { EventTrigger } from "@/hooks/api/types";

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

  return (
    <Card>
      <CardContent className="space-y-3">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-2">
            <StatusBadge status={event.status} />
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
            Source:{" "}
            <Badge mono size="xs" variant="secondary-light">
              {event.source_type}
            </Badge>
          </span>
          <span>
            Type:{" "}
            <Badge mono size="xs" variant="secondary-light">
              {event.trigger_type}
            </Badge>
          </span>
          {event.job_run_id && (
            <span>
              Run:{" "}
              <Badge mono size="xs" variant="secondary-light">
                {event.job_run_id}
              </Badge>
            </span>
          )}
        </div>
        {event.request_payload != null && (
          <CodeBlock
            code={JSON.stringify(event.request_payload, null, 2)}
            language="json"
            maxHeight={300}
          />
        )}
        {event.error && (
          <Alert variant="destructive">
            <AlertDescription>{event.error}</AlertDescription>
          </Alert>
        )}
      </CardContent>
    </Card>
  );
};

export default ExpandedEventDetail;
