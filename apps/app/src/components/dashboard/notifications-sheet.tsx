import { HugeiconsIcon } from "@hugeicons/react";
import { Button } from "@strait/ui/components/button";
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
  SheetTrigger,
} from "@strait/ui/components/sheet";
import { cn } from "@strait/ui/utils/index";
import { useState } from "react";
import {
  AlertIcon,
  BellIcon,
  CheckCircleIcon,
  CheckIcon,
  InfoIcon,
  TrashIcon,
  XCircleIcon,
} from "@/lib/icons";

type NotificationType = "success" | "error" | "warning" | "info";

type Notification = {
  id: string;
  type: NotificationType;
  title: string;
  message: string;
  timestamp: string;
  read: boolean;
};

const TYPE_ICONS: Record<NotificationType, any> = {
  success: CheckCircleIcon,
  error: XCircleIcon,
  warning: AlertIcon,
  info: InfoIcon,
};

const TYPE_COLORS: Record<NotificationType, string> = {
  success: "text-chart-1",
  error: "text-chart-4",
  warning: "text-chart-3",
  info: "text-chart-2",
};

const INITIAL_NOTIFICATIONS: Notification[] = [
  {
    id: "1",
    type: "error",
    title: "Job Failed",
    message: "report-gen failed after 3 attempts",
    timestamp: "2 min ago",
    read: false,
  },
  {
    id: "2",
    type: "success",
    title: "Workflow Complete",
    message: "onboarding-flow completed successfully",
    timestamp: "10 min ago",
    read: false,
  },
  {
    id: "3",
    type: "warning",
    title: "High Queue Depth",
    message: "Queue depth exceeded 500 for payment-sync",
    timestamp: "25 min ago",
    read: true,
  },
  {
    id: "4",
    type: "info",
    title: "Scheduled Maintenance",
    message: "System maintenance window at 02:00 UTC",
    timestamp: "1 hour ago",
    read: true,
  },
];

export function NotificationsSheet() {
  const [notifications, setNotifications] = useState(INITIAL_NOTIFICATIONS);
  const unreadCount = notifications.filter((n) => !n.read).length;

  function markAllRead() {
    setNotifications((prev) => prev.map((n) => ({ ...n, read: true })));
  }

  function clearAll() {
    setNotifications([]);
  }

  return (
    <Sheet>
      <SheetTrigger
        render={<Button className="relative" size="icon" variant="ghost" />}
      >
        <HugeiconsIcon icon={BellIcon} size={18} />
        {unreadCount > 0 && (
          <span className="absolute -top-0.5 -right-0.5 flex size-4 items-center justify-center rounded-full bg-destructive font-medium text-destructive-foreground text-xs">
            {unreadCount}
          </span>
        )}
      </SheetTrigger>
      <SheetContent>
        <SheetHeader>
          <SheetTitle>Notifications</SheetTitle>
        </SheetHeader>
        <div className="mt-4 flex items-center justify-between">
          <span className="text-muted-foreground text-xs">
            {unreadCount} unread
          </span>
          <div className="flex gap-1">
            <Button
              disabled={unreadCount === 0}
              onClick={markAllRead}
              variant="ghost"
            >
              <HugeiconsIcon className="mr-1" icon={CheckIcon} size={12} />
              Mark all read
            </Button>
            <Button
              disabled={notifications.length === 0}
              onClick={clearAll}
              variant="ghost"
            >
              <HugeiconsIcon className="mr-1" icon={TrashIcon} size={12} />
              Clear all
            </Button>
          </div>
        </div>
        <div className="mt-4 space-y-1">
          {notifications.length === 0 && (
            <p className="py-8 text-center text-muted-foreground text-sm">
              No notifications
            </p>
          )}
          {notifications.map((notif) => (
            <div
              className={cn(
                "flex gap-3 rounded-md p-3 transition-colors",
                !notif.read && "bg-muted/50"
              )}
              key={notif.id}
            >
              <HugeiconsIcon
                className={cn("mt-0.5 shrink-0", TYPE_COLORS[notif.type])}
                icon={TYPE_ICONS[notif.type]}
                size={16}
              />
              <div className="min-w-0 flex-1">
                <p className="font-medium text-sm">{notif.title}</p>
                <p className="text-muted-foreground text-xs">{notif.message}</p>
                <p className="mt-1 text-muted-foreground text-xs">
                  {notif.timestamp}
                </p>
              </div>
              {!notif.read && (
                <span className="mt-1.5 size-2 shrink-0 rounded-full bg-chart-2" />
              )}
            </div>
          ))}
        </div>
      </SheetContent>
    </Sheet>
  );
}
