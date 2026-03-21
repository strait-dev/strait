import { Button } from "@strait/ui/components/button";
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@strait/ui/components/card";
import { useNavigate } from "@tanstack/react-router";
import { useCallback } from "react";

type LimitReachedErrorProps = {
  error: {
    message: string;
    limit?: number;
    current_usage?: number;
    plan?: string;
  };
};

const LimitReachedError = ({ error }: LimitReachedErrorProps) => {
  const navigate = useNavigate();

  const handleUpgrade = useCallback(() => {
    navigate({ to: "/app/upgrade" });
  }, [navigate]);

  return (
    <Card className="border-destructive/50">
      <CardHeader className="pb-3">
        <CardTitle className="font-medium text-destructive text-sm">
          Rate Limit Reached
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-3">
        <p className="text-muted-foreground text-sm">{error.message}</p>

        {error.current_usage !== undefined && error.limit !== undefined ? (
          <div className="flex items-center justify-between rounded-md border bg-muted/50 px-3 py-2">
            <span className="text-muted-foreground text-xs">Usage</span>
            <span className="font-mono text-sm tabular-nums">
              {error.current_usage.toLocaleString()} /{" "}
              {error.limit.toLocaleString()}
            </span>
          </div>
        ) : null}

        {error.plan ? (
          <p className="text-muted-foreground text-xs">
            Current plan: <span className="font-medium">{error.plan}</span>
          </p>
        ) : null}

        <Button onClick={handleUpgrade} size="sm" variant="default">
          Upgrade Plan
        </Button>
      </CardContent>
    </Card>
  );
};

export default LimitReachedError;
