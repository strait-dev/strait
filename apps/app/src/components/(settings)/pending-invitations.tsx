import { HugeiconsIcon } from "@hugeicons/react";
import { Badge } from "@strait/ui/components/badge";
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
import {
  useAcceptInvitation,
  useRejectInvitation,
  userInvitationsQueryOptions,
} from "@/hooks/auth/use-invitation";
import { CheckIcon, LoadingIcon, MailIcon, TrashIcon } from "@/lib/icons";

const PendingInvitations = () => {
  const { data: invitations, isLoading } = useQuery(
    userInvitationsQueryOptions()
  );
  const acceptInvitation = useAcceptInvitation();
  const rejectInvitation = useRejectInvitation();

  const handleAccept = async (invitationId: string) => {
    try {
      await acceptInvitation.mutateAsync(invitationId);
      toast.success("Invitation accepted!");
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : "Failed to accept invitation."
      );
    }
  };

  const handleReject = async (invitationId: string) => {
    try {
      await rejectInvitation.mutateAsync(invitationId);
      toast.success("Invitation rejected.");
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : "Failed to reject invitation."
      );
    }
  };

  if (isLoading) {
    return null;
  }

  if (!invitations || invitations.length === 0) {
    return null;
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>
          <div className="flex items-center gap-2">
            <HugeiconsIcon className="size-4" icon={MailIcon} />
            Organization Invitations
          </div>
        </CardTitle>
        <CardDescription>
          You have pending invitations to join organizations.
        </CardDescription>
      </CardHeader>
      <CardContent>
        <div className="flex flex-col gap-3">
          {invitations.map((invitation) => {
            const isAccepting =
              acceptInvitation.isPending &&
              acceptInvitation.variables === invitation.id;
            const isRejecting =
              rejectInvitation.isPending &&
              rejectInvitation.variables === invitation.id;

            return (
              <div
                className="flex flex-col gap-3 rounded-md border p-3 sm:flex-row sm:items-center sm:justify-between"
                key={invitation.id}
              >
                <div className="flex flex-col gap-1">
                  <p className="font-medium text-sm">
                    {invitation.organizationName ?? "Unknown organization"}
                  </p>
                  <div className="flex items-center gap-2">
                    <Badge variant="outline">{invitation.role}</Badge>
                    <span className="text-muted-foreground text-xs">
                      Invited to {invitation.email}
                    </span>
                  </div>
                </div>
                <div className="flex w-full gap-2 sm:w-auto">
                  <Button
                    disabled={isRejecting || isAccepting}
                    onClick={() => handleReject(invitation.id)}
                    variant="outline"
                  >
                    {isRejecting ? (
                      <HugeiconsIcon
                        className="size-3 animate-spin"
                        icon={LoadingIcon}
                      />
                    ) : (
                      <HugeiconsIcon className="size-3" icon={TrashIcon} />
                    )}
                    Reject
                  </Button>
                  <Button
                    disabled={isAccepting || isRejecting}
                    onClick={() => handleAccept(invitation.id)}
                  >
                    {isAccepting ? (
                      <HugeiconsIcon
                        className="size-3 animate-spin"
                        icon={LoadingIcon}
                      />
                    ) : (
                      <HugeiconsIcon className="size-3" icon={CheckIcon} />
                    )}
                    Accept
                  </Button>
                </div>
              </div>
            );
          })}
        </div>
      </CardContent>
    </Card>
  );
};

export default PendingInvitations;
