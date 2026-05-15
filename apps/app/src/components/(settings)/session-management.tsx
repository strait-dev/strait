import { HugeiconsIcon } from "@hugeicons/react";
import { Button } from "@strait/ui/components/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@strait/ui/components/card";
import { toast } from "@strait/ui/components/toast/index";
import { useQuery } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import {
  sessionsQueryOptions,
  useRevokeAllSessions,
  useRevokeOtherSessions,
  useRevokeSession,
} from "@/hooks/auth/use-account";
import { GlobeIcon, LoadingIcon, LogOutIcon } from "@/lib/icons";

const SessionManagement = () => {
  const navigate = useNavigate();
  const { data, isLoading } = useQuery(sessionsQueryOptions());
  const revokeSession = useRevokeSession();
  const revokeOtherSessions = useRevokeOtherSessions();
  const revokeAllSessions = useRevokeAllSessions();

  const sessions = data?.sessions ?? [];
  const currentToken = data?.currentToken ?? null;

  const handleRevoke = async (token: string) => {
    try {
      await revokeSession.mutateAsync(token);
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
                {revokeOtherSessions.isPending ? (
                  <HugeiconsIcon
                    className="size-3 animate-spin"
                    icon={LoadingIcon}
                  />
                ) : null}
                Revoke all others
              </Button>
            )}
            <Button
              disabled={revokeAllSessions.isPending}
              onClick={handleSignOutEverywhere}
              variant="destructive"
            >
              {revokeAllSessions.isPending ? (
                <HugeiconsIcon
                  className="size-3 animate-spin"
                  icon={LoadingIcon}
                />
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
            <HugeiconsIcon className="size-4 animate-spin" icon={LoadingIcon} />
            Loading sessions...
          </div>
        )}
        {!isLoading && sessions.length === 0 && (
          <p className="text-muted-foreground text-sm">No active sessions.</p>
        )}
        {!isLoading && sessions.length > 0 && (
          <div className="flex flex-col gap-3">
            {sessions.map((session) => {
              const isCurrent = session.token === currentToken;
              const isRevoking =
                revokeSession.isPending &&
                revokeSession.variables === session.token;

              return (
                <div
                  className="flex items-center justify-between rounded-md border p-3"
                  key={session.id}
                >
                  <div className="flex items-center gap-3">
                    <HugeiconsIcon
                      className="size-4 text-muted-foreground"
                      icon={GlobeIcon}
                    />
                    <div>
                      <p className="font-medium text-sm">
                        {parseUserAgent(session.userAgent)}
                        {isCurrent && (
                          <span className="ml-2 text-muted-foreground text-xs">
                            (current)
                          </span>
                        )}
                      </p>
                      <p className="text-muted-foreground text-xs">
                        {session.ipAddress ?? "Unknown IP"} &middot; Last active{" "}
                        {formatDate(session.updatedAt)}
                      </p>
                    </div>
                  </div>
                  {!isCurrent && (
                    <Button
                      disabled={isRevoking}
                      onClick={() => handleRevoke(session.token)}
                      variant="outline"
                    >
                      {isRevoking ? (
                        <HugeiconsIcon
                          className="size-3 animate-spin"
                          icon={LoadingIcon}
                        />
                      ) : null}
                      Revoke
                    </Button>
                  )}
                </div>
              );
            })}
          </div>
        )}
      </CardContent>
    </Card>
  );
};

export default SessionManagement;
