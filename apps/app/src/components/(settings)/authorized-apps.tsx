import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from "@strait/ui/components/alert-dialog";
import { Badge } from "@strait/ui/components/badge";
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
import { Frame, FramePanel } from "@strait/ui/components/frame";
import { IdCell } from "@strait/ui/components/id-cell";
import { Skeleton } from "@strait/ui/components/skeleton";
import { toast } from "@strait/ui/components/toast";
import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import type { OAuthConsentItem } from "@/hooks/api/use-oauth-consents";
import {
  oauthConsentsQueryOptions,
  useRevokeOAuthConsent,
} from "@/hooks/api/use-oauth-consents";

export function AuthorizedApps() {
  const [revokingId, setRevokingId] = useState<string | null>(null);
  const { data: consents = [], isLoading } = useQuery(
    oauthConsentsQueryOptions()
  );
  const revokeMutation = useRevokeOAuthConsent();

  function handleRevoke(consentId: string) {
    setRevokingId(consentId);
    revokeMutation.mutate(consentId, {
      onSuccess: () => {
        toast.success("Access revoked successfully");
        setRevokingId(null);
      },
      onError: () => {
        toast.error("Failed to revoke access");
        setRevokingId(null);
      },
    });
  }

  if (isLoading) {
    return (
      <Card>
        <CardHeader>
          <CardTitle>Authorized apps</CardTitle>
          <CardDescription>
            Applications that have access to your account via OAuth.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div className="flex flex-col gap-3">
            <Skeleton className="h-16 w-full" />
            <Skeleton className="h-16 w-full" />
          </div>
        </CardContent>
      </Card>
    );
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>Authorized apps</CardTitle>
        <CardDescription>
          Applications that have access to your account via OAuth. Revoking
          access will immediately invalidate all tokens for that application.
        </CardDescription>
      </CardHeader>
      <CardContent>
        {consents.length === 0 ? (
          <Empty border={false} className="py-4">
            <EmptyHeader>
              <EmptyTitle>No authorized applications</EmptyTitle>
              <EmptyDescription>
                OAuth applications you approve will appear here.
              </EmptyDescription>
            </EmptyHeader>
          </Empty>
        ) : (
          <div className="flex flex-col gap-3">
            {consents.map((consent) => (
              <ConsentRow
                consent={consent}
                key={consent.id}
                onRevoke={() => handleRevoke(consent.id)}
                revoking={revokingId === consent.id}
              />
            ))}
          </div>
        )}
      </CardContent>
    </Card>
  );
}

function ConsentRow({
  consent,
  onRevoke,
  revoking,
}: {
  consent: OAuthConsentItem;
  onRevoke: () => void;
  revoking: boolean;
}) {
  const scopes = consent.scopes
    .split(",")
    .map((s) => s.trim())
    .filter(Boolean);

  const grantedAt = new Date(consent.createdAt).toLocaleDateString(undefined, {
    year: "numeric",
    month: "short",
    day: "numeric",
  });

  return (
    <Frame>
      <FramePanel className="flex items-start justify-between gap-4">
        <div className="flex flex-col gap-2">
          <div className="flex flex-col gap-0.5">
            <div className="flex items-center gap-2">
              <span className="font-medium text-foreground text-sm">
                {consent.clientName}
              </span>
              <span className="text-muted-foreground text-xs">
                Authorized {grantedAt}
              </span>
            </div>
            <IdCell id={consent.clientId} length={8} />
          </div>
          <div className="flex flex-wrap gap-1">
            {scopes.map((scope) => (
              <Badge key={scope} size="xs" variant="secondary">
                {scope}
              </Badge>
            ))}
          </div>
        </div>
        <AlertDialog>
          <AlertDialogTrigger
            render={
              <Button disabled={revoking} variant="destructive">
                {revoking ? "Revoking..." : "Revoke"}
              </Button>
            }
          />
          <AlertDialogContent>
            <AlertDialogHeader>
              <AlertDialogTitle>Revoke access</AlertDialogTitle>
              <AlertDialogDescription>
                This will immediately revoke all access tokens for this
                application. The application will need to request authorization
                again.
              </AlertDialogDescription>
            </AlertDialogHeader>
            <div className="flex justify-end gap-3">
              <AlertDialogCancel
                render={<Button variant="outline">Cancel</Button>}
              />
              <AlertDialogAction
                onClick={onRevoke}
                render={<Button variant="destructive">Revoke access</Button>}
              />
            </div>
          </AlertDialogContent>
        </AlertDialog>
      </FramePanel>
    </Frame>
  );
}
