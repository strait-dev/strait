import { HugeiconsIcon } from "@hugeicons/react";
import { Badge } from "@strait/ui/components/badge";
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
} from "@strait/ui/components/sheet";
import { formatDistanceToNow } from "date-fns";

import StatusBadge from "@/components/dashboard/status-badge";
import type { WebhookSubscription } from "@/hooks/api/types";
import { GlobeIcon, WebhookIcon } from "@/lib/icons";

const WebhookDetailSheet = ({
  webhook,
  open,
  onOpenChange,
}: {
  webhook: WebhookSubscription | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) => {
  if (!webhook) {
    return null;
  }

  return (
    <Sheet onOpenChange={onOpenChange} open={open}>
      <SheetContent className="flex flex-col overflow-y-auto">
        <SheetHeader>
          <SheetTitle className="flex items-center gap-2">
            <HugeiconsIcon
              className="text-muted-foreground"
              icon={WebhookIcon}
              size={16}
            />
            Webhook Details
          </SheetTitle>
        </SheetHeader>

        <div className="mt-4 flex-1 space-y-6 overflow-y-auto px-6">
          {/* Status */}
          <div className="flex items-center gap-2">
            <StatusBadge status={webhook.active ? "completed" : "pending"} />
          </div>

          {/* Endpoint */}
          <div>
            <h4 className="mb-2 font-medium text-muted-foreground text-xs uppercase">
              Endpoint
            </h4>
            <div className="flex items-center gap-2">
              <HugeiconsIcon
                className="shrink-0 text-muted-foreground"
                icon={GlobeIcon}
                size={14}
              />
              <code className="min-w-0 break-all text-xs">
                {webhook.webhook_url}
              </code>
            </div>
          </div>

          {/* Event Types */}
          <div>
            <h4 className="mb-2 font-medium text-muted-foreground text-xs uppercase">
              Subscribed Events
            </h4>
            <div className="flex flex-wrap gap-1.5">
              {(webhook.event_types ?? []).map((event) => (
                <Badge key={event} variant="secondary">
                  {event}
                </Badge>
              ))}
            </div>
          </div>

          {/* Metadata */}
          <div>
            <h4 className="mb-2 font-medium text-muted-foreground text-xs uppercase">
              Metadata
            </h4>
            <div className="space-y-1.5 text-sm">
              <div className="flex items-center justify-between gap-2">
                <span className="shrink-0 text-muted-foreground">ID</span>
                <code className="truncate text-xs">{webhook.id}</code>
              </div>
              <div className="flex justify-between">
                <span className="text-muted-foreground">Created</span>
                <span className="text-xs">
                  {formatDistanceToNow(new Date(webhook.created_at), {
                    addSuffix: true,
                  })}
                </span>
              </div>
            </div>
          </div>
        </div>
      </SheetContent>
    </Sheet>
  );
};

export default WebhookDetailSheet;
