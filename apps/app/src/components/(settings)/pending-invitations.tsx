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
import {
  Item,
  ItemActions,
  ItemContent,
  ItemDescription,
  ItemGroup,
  ItemTitle,
} from "@strait/ui/components/item";
import { Spinner } from "@strait/ui/components/spinner";
import { toast } from "@strait/ui/components/toast";
import { useQuery } from "@tanstack/react-query";
import {
  useAcceptInvitation,
  useRejectInvitation,
  userInvitationsQueryOptions,
} from "@/hooks/auth/use-invitation";
import { CheckIcon, MailIcon, TrashIcon } from "@/lib/icons";

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
        <ItemGroup>
          {invitations.map((invitation) => {
            const isAccepting =
              acceptInvitation.isPending &&
              acceptInvitation.variables === invitation.id;
            const isRejecting =
              rejectInvitation.isPending &&
              rejectInvitation.variables === invitation.id;

            return (
              <Item key={invitation.id} variant="outline">
                <ItemContent>
                  <ItemTitle>
                    {invitation.organizationName ?? "Unknown organization"}
                  </ItemTitle>
                  <ItemDescription className="flex items-center gap-2">
                    <Badge variant="outline">{invitation.role}</Badge>
                    Invited to {invitation.email}
                  </ItemDescription>
                </ItemContent>
                <ItemActions className="w-full sm:w-auto">
                  <Button
                    disabled={isRejecting || isAccepting}
                    onClick={() => handleReject(invitation.id)}
                    variant="outline"
                  >
                    {isRejecting ? (
                      <Spinner size="xs" />
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
                      <Spinner size="xs" />
                    ) : (
                      <HugeiconsIcon className="size-3" icon={CheckIcon} />
                    )}
                    Accept
                  </Button>
                </ItemActions>
              </Item>
            );
          })}
        </ItemGroup>
      </CardContent>
    </Card>
  );
};

export default PendingInvitations;
