import { Badge } from "@strait/ui/components/badge";
import { cn } from "@strait/ui/utils/index";
import { formatDistanceToNow } from "date-fns";

import type { EventTrigger } from "@/hooks/api/types";
import { EVENT_STATUS_STYLES } from "@/lib/status";

const EventRow = ({ event }: { event: EventTrigger }) => {
  const style = EVENT_STATUS_STYLES[event.status] ?? EVENT_STATUS_STYLES.pending;

  return (
    <div className="relative flex items-start gap-3 py-2.5 pl-0">
      {/* Dot */}
      <span
        className={cn(
          "relative z-10 mt-1.5 h-[9px] w-[9px] shrink-0 rounded-full border-2 border-background",
          style.dot
        )}
      />

      {/* Content */}
      <div className="flex min-w-0 flex-1 flex-col gap-0.5">
        <div className="flex items-center gap-2">
          <Badge className={cn("px-1.5 py-0", style.badge)} variant="outline">
            {style.label}
          </Badge>
          <span className="font-mono text-muted-foreground text-xs">
            {formatDistanceToNow(new Date(event.requested_at), {
              addSuffix: true,
            })}
          </span>
        </div>
        <p className="text-sm">{event.event_key}</p>
        <span className="font-mono text-muted-foreground text-xs">
          {event.trigger_type} | {event.source_type}
        </span>
      </div>
    </div>
  );
};

export default EventRow;
