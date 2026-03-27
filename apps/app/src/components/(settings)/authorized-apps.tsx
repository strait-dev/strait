import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import type { OAuthConsentItem } from "@/hooks/api/use-oauth-consents";
import {
  oauthConsentsQueryOptions,
  useRevokeOAuthConsent,
} from "@/hooks/api/use-oauth-consents";
import { Button } from "@strait/ui/components/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@strait/ui/components/card";
import { Badge } from "@strait/ui/components/badge";
import { toast } from "@strait/ui/components/toast";
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
          <CardTitle>Authorized Apps</CardTitle>
          <CardDescription>
            Applications that have access to your account via OAuth.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div className="flex items-center justify-center py-8">
            <div className="h-5 w-5 animate-spin rounded-full border-2 border-muted-foreground/30 border-t-primary" />
          </div>
        </CardContent>
      </Card>
    );
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>Authorized Apps</CardTitle>
        <CardDescription>
          Applications that have access to your account via OAuth. Revoking
          access will immediately invalidate all tokens for that application.
        </CardDescription>
      </CardHeader>
      <CardContent>
        {consents.length === 0 ? (
          <p className="py-4 text-center text-muted-foreground text-sm">
            No applications have been authorized yet.
          </p>
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
    <div className="flex items-start justify-between gap-4 rounded-lg border border-border p-4">
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
          <span className="font-mono text-muted-foreground text-xs">
            {consent.clientId}
          </span>
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
            <Button disabled={revoking} size="sm" variant="destructive">
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
              render={<Button variant="destructive">Revoke Access</Button>}
            />
          </div>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}
