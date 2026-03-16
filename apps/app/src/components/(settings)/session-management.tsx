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
import { useCallback, useEffect, useState } from "react";
import { authClient } from "@/lib/auth-client";
import { GlobeIcon, LoadingIcon, LogOutIcon } from "@/lib/icons";
import { captureException } from "@/lib/sentry";

type Session = {
  id: string;
  token: string;
  createdAt: string | Date;
  updatedAt: string | Date;
  ipAddress: string | null;
  userAgent: string | null;
};

const SessionManagement = () => {
  const [sessions, setSessions] = useState<Session[]>([]);
  const [currentToken, setCurrentToken] = useState<string | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [revokingToken, setRevokingToken] = useState<string | null>(null);
  const [isRevokingAll, setIsRevokingAll] = useState(false);
  const [isSigningOutAll, setIsSigningOutAll] = useState(false);

  const fetchSessions = useCallback(async () => {
    try {
      const [sessionsResult, sessionResult] = await Promise.all([
        authClient.listSessions(),
        authClient.getSession(),
      ]);
      if (sessionsResult.data) {
        setSessions(sessionsResult.data as unknown as Session[]);
      }
      if (sessionResult.data?.session) {
        setCurrentToken(sessionResult.data.session.token);
      }
    } catch (error) {
      captureException(error);
    } finally {
      setIsLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchSessions();
  }, [fetchSessions]);

  const handleRevoke = async (token: string) => {
    setRevokingToken(token);
    try {
      const result = await authClient.revokeSession({ token });

      if (result.error) {
        toast.error(result.error.message ?? "Failed to revoke session.");
        setRevokingToken(null);
        return;
      }

      toast.success("Session revoked.");
      await fetchSessions();
    } catch (error) {
      captureException(error);
      toast.error("Failed to revoke session.");
    } finally {
      setRevokingToken(null);
    }
  };

  const handleRevokeAll = async () => {
    setIsRevokingAll(true);
    try {
      const result = await authClient.revokeOtherSessions();

      if (result.error) {
        toast.error(result.error.message ?? "Failed to revoke other sessions.");
        setIsRevokingAll(false);
        return;
      }

      toast.success("All other sessions revoked.");
      await fetchSessions();
    } catch (error) {
      captureException(error);
      toast.error("Failed to revoke sessions.");
    } finally {
      setIsRevokingAll(false);
    }
  };

  const handleSignOutEverywhere = async () => {
    setIsSigningOutAll(true);
    try {
      const result = await authClient.revokeSessions();

      if (result.error) {
        toast.error(
          result.error.message ?? "Failed to sign out of all sessions."
        );
        setIsSigningOutAll(false);
        return;
      }

      window.location.href = "/login";
    } catch (error) {
      captureException(error);
      toast.error("Failed to sign out of all sessions.");
      setIsSigningOutAll(false);
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

  const formatDate = (date: string | Date) => {
    return new Date(date).toLocaleDateString("en-US", {
      year: "numeric",
      month: "short",
      day: "numeric",
      hour: "2-digit",
      minute: "2-digit",
    });
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
                disabled={isRevokingAll}
                onClick={handleRevokeAll}
                size="sm"
                variant="outline"
              >
                {isRevokingAll ? (
                  <HugeiconsIcon
                    className="size-3 animate-spin"
                    icon={LoadingIcon}
                  />
                ) : null}
                Revoke all others
              </Button>
            )}
            <Button
              disabled={isSigningOutAll}
              onClick={handleSignOutEverywhere}
              size="sm"
              variant="destructive"
            >
              {isSigningOutAll ? (
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
                      disabled={revokingToken === session.token}
                      onClick={() => handleRevoke(session.token)}
                      size="sm"
                      variant="outline"
                    >
                      {revokingToken === session.token ? (
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
