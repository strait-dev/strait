import { HugeiconsIcon } from "@hugeicons/react";
import { Button } from "@strait/ui/components/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@strait/ui/components/card";
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyTitle,
} from "@strait/ui/components/empty";
import {
  Item,
  ItemActions,
  ItemContent,
  ItemDescription,
  ItemGroup,
  ItemMedia,
  ItemTitle,
} from "@strait/ui/components/item";
import { Spinner } from "@strait/ui/components/spinner";
import { toast } from "@strait/ui/components/toast";
import { useQuery } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import {
  sessionsQueryOptions,
  useRevokeAllSessions,
  useRevokeOtherSessions,
  useRevokeSession,
} from "@/hooks/auth/use-account";
import { GlobeIcon, LogOutIcon } from "@/lib/icons";

const parseUserAgent = (ua: string | null): string => {
  if (!ua) {
    return "Unknown device";
  }
  if (ua.includes("Chrome")) {
    return "Chrome";
  }
  if (ua.includes("Firefox")) {
    return "Firefox";
  }
  if (ua.includes("Safari")) {
    return "Safari";
  }
  if (ua.includes("Edge")) {
    return "Edge";
  }
  return "Unknown browser";
};

const formatDate = (date: string | Date) =>
  new Date(date).toLocaleDateString("en-US", {
    year: "numeric",
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  });

const SessionManagement = () => {
  const navigate = useNavigate();
  const { data, isLoading } = useQuery(sessionsQueryOptions());
  const revokeSession = useRevokeSession();
  const revokeOtherSessions = useRevokeOtherSessions();
  const revokeAllSessions = useRevokeAllSessions();

  const sessions = data?.sessions ?? [];

  const handleRevoke = async (sessionId: string) => {
    try {
      await revokeSession.mutateAsync(sessionId);
      toast.success("Session revoked.");
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : "Failed to revoke session."
      );
    }
  };

  const handleRevokeAll = async () => {
    try {
      await revokeOtherSessions.mutateAsync();
      toast.success("All other sessions revoked.");
    } catch (error) {
      toast.error(
        error instanceof Error
          ? error.message
          : "Failed to revoke other sessions."
      );
    }
  };

  const handleSignOutEverywhere = async () => {
    try {
      await revokeAllSessions.mutateAsync();
      await navigate({ to: "/login", replace: true });
    } catch (error) {
      toast.error(
        error instanceof Error
          ? error.message
          : "Failed to sign out of all sessions."
      );
    }
  };

  return (
    <Card>
      <CardHeader>
        <div className="flex items-center justify-between">
          <div>
            <CardTitle>Active sessions</CardTitle>
            <CardDescription>
              Manage your active sessions across devices.
            </CardDescription>
          </div>
          <div className="flex gap-2">
            {sessions.length > 1 && (
              <Button
                disabled={revokeOtherSessions.isPending}
                onClick={handleRevokeAll}
                variant="outline"
              >
                {revokeOtherSessions.isPending ? <Spinner size="xs" /> : null}
                Revoke all others
              </Button>
            )}
            <Button
              disabled={revokeAllSessions.isPending}
              onClick={handleSignOutEverywhere}
              variant="destructive"
            >
              {revokeAllSessions.isPending ? (
                <Spinner size="xs" />
              ) : (
                <HugeiconsIcon className="size-3" icon={LogOutIcon} />
              )}
              Sign out everywhere
            </Button>
          </div>
        </div>
      </CardHeader>
      <CardContent>
        {isLoading && (
          <div className="flex items-center gap-2 text-muted-foreground text-sm">
            <Spinner />
            Loading sessions...
          </div>
        )}
        {!isLoading && sessions.length === 0 && (
          <Empty border={false} className="py-4">
            <EmptyHeader>
              <EmptyTitle>No active sessions</EmptyTitle>
              <EmptyDescription>
                Active browser sessions will appear here when you sign in.
              </EmptyDescription>
            </EmptyHeader>
          </Empty>
        )}
        {!isLoading && sessions.length > 0 && (
          <ItemGroup>
            {sessions.map((session) => {
              const isCurrent = session.isCurrent;
              const isRevoking =
                revokeSession.isPending &&
                revokeSession.variables === session.id;

              return (
                <Item key={session.id} variant="outline">
                  <ItemMedia variant="icon">
                    <HugeiconsIcon
                      className="size-4 text-muted-foreground"
                      icon={GlobeIcon}
                    />
                  </ItemMedia>
                  <ItemContent>
                    <ItemTitle>
                      {parseUserAgent(session.userAgent)}
                      {isCurrent && (
                        <span className="ml-2 text-muted-foreground text-xs">
                          (current)
                        </span>
                      )}
                    </ItemTitle>
                    <ItemDescription>
                      {session.ipAddress ?? "Unknown IP"} &middot; Last active{" "}
                      {formatDate(session.updatedAt)}
                    </ItemDescription>
                  </ItemContent>
                  {!isCurrent && (
                    <ItemActions>
                      <Button
                        disabled={isRevoking}
                        onClick={() => handleRevoke(session.id)}
                        variant="outline"
                      >
                        {isRevoking ? <Spinner size="xs" /> : null}
                        Revoke
                      </Button>
                    </ItemActions>
                  )}
                </Item>
              );
            })}
          </ItemGroup>
        )}
      </CardContent>
    </Card>
  );
};

export default SessionManagement;
