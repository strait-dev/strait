import { createServerFn } from "@tanstack/react-start";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { z } from "zod";
import { auth } from "@/lib/auth.server";
import { authMiddleware } from "@/middlewares/auth";
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

// -- Server functions ---------------------------------------------------------

type OAuthConsentItem = {
  id: string;
  clientId: string;
  scopes: string;
  createdAt: string;
  updatedAt: string;
};

const fetchConsents = createServerFn({ method: "GET" })
  .middleware([authMiddleware])
  .handler(async () => {
    const consents = await auth.api.getOAuthConsents();
    return (consents ?? []) as unknown as OAuthConsentItem[];
  });

const revokeConsent = createServerFn({ method: "POST" })
  .inputValidator(z.object({ consentId: z.string().min(1) }))
  .middleware([authMiddleware])
  .handler(async ({ data }) => {
    await auth.api.deleteOAuthConsent({
      body: { id: data.consentId },
    });
    return { success: true };
  });

// -- Query key ----------------------------------------------------------------

const AUTHORIZED_APPS_KEY = ["authorized-apps"] as const;

// -- Component ----------------------------------------------------------------

export function AuthorizedApps() {
  const queryClient = useQueryClient();
  const [revokingId, setRevokingId] = useState<string | null>(null);

  const { data: consents = [], isLoading } = useQuery({
    queryKey: AUTHORIZED_APPS_KEY,
    queryFn: () => fetchConsents(),
  });

  const revokeMutation = useMutation({
    mutationFn: (consentId: string) =>
      revokeConsent({ data: { consentId } }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: AUTHORIZED_APPS_KEY });
      toast.success("Access revoked successfully");
      setRevokingId(null);
    },
    onError: () => {
      toast.error("Failed to revoke access");
      setRevokingId(null);
    },
  });

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
                onRevoke={() => {
                  setRevokingId(consent.id);
                  revokeMutation.mutate(consent.id);
                }}
                revoking={revokingId === consent.id}
              />
            ))}
          </div>
        )}
      </CardContent>
    </Card>
  );
}

// -- Consent row --------------------------------------------------------------

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
        <div className="flex items-center gap-2">
          <span className="font-medium text-foreground text-sm">
            {consent.clientId}
          </span>
          <span className="text-muted-foreground text-xs">
            Authorized {grantedAt}
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
            <Button
              disabled={revoking}
              size="sm"
              variant="destructive"
            >
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
            <AlertDialogCancel render={<Button variant="outline">Cancel</Button>} />
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
