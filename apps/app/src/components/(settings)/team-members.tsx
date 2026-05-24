import { HugeiconsIcon } from "@hugeicons/react";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from "@strait/ui/components/alert-dialog";
import {
  Avatar,
  AvatarFallback,
  AvatarImage,
} from "@strait/ui/components/avatar";
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
import { useNavigate } from "@tanstack/react-router";
import ChangeRoleDropdown from "@/components/(settings)/change-role-dropdown";
import InviteMemberDialog from "@/components/(settings)/invite-member-dialog";
import type { InvitationData } from "@/hooks/auth/use-invitation";
import {
  invitationsQueryOptions,
  useCancelInvitation,
  useCreateInvitation,
} from "@/hooks/auth/use-invitation";
import type { MemberData } from "@/hooks/auth/use-member";
import {
  membersQueryOptions,
  useLeaveOrganization,
  useRemoveMember,
} from "@/hooks/auth/use-member";
import { useOrganizationRole } from "@/hooks/auth/use-permissions";
import {
  LoadingIcon,
  LogOutIcon,
  MailIcon,
  RefreshIcon,
  TrashIcon,
} from "@/lib/icons";

interface TeamMembersProps {
  currentUserId: string;
  organizationId: string;
}

const formatDate = (date: Date | string) =>
  new Date(date).toLocaleDateString("en-US", {
    year: "numeric",
    month: "short",
    day: "numeric",
  });

const getInitials = (name: string) =>
  name
    .split(" ")
    .map((n) => n[0])
    .join("")
    .toUpperCase()
    .slice(0, 2);

const roleBadgeVariant = (role: string) => {
  switch (role) {
    case "owner":
      return "default" as const;
    case "admin":
      return "secondary" as const;
    default:
      return "outline" as const;
  }
};

const TeamMembers = ({ organizationId, currentUserId }: TeamMembersProps) => {
  const { data: members, isLoading: membersLoading } = useQuery(
    membersQueryOptions({ organizationId })
  );

  const { data: invitations, isLoading: invitationsLoading } = useQuery(
    invitationsQueryOptions({ organizationId })
  );

  const cancelInvitation = useCancelInvitation();
  const removeMember = useRemoveMember();
  const leaveOrganization = useLeaveOrganization();
  const createInvitation = useCreateInvitation();
  const navigate = useNavigate();
  const { isOwner, isAdmin } = useOrganizationRole(
    organizationId,
    currentUserId
  );

  const pendingInvitations = (invitations ?? []).filter(
    (inv: InvitationData) => inv.status === "pending"
  );

  const handleCancelInvitation = async (invitationId: string) => {
    try {
      await cancelInvitation.mutateAsync({ invitationId });
      toast.success("Invitation cancelled.");
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : "Failed to cancel invitation."
      );
    }
  };

  const handleRemoveMember = async (memberIdOrEmail: string) => {
    try {
      await removeMember.mutateAsync({ memberIdOrEmail, organizationId });
      toast.success("Member removed.");
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : "Failed to remove member."
      );
    }
  };

  const handleLeaveOrganization = async (memberId: string) => {
    try {
      await leaveOrganization.mutateAsync({
        memberIdOrEmail: memberId,
        organizationId,
      });
      toast.success("You have left the organization.");
      navigate({ to: "/app" });
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : "Failed to leave organization."
      );
    }
  };

  const handleResendInvitation = async (email: string, role: string) => {
    try {
      await createInvitation.mutateAsync({
        email,
        role: role as "member" | "admin" | "owner",
        organizationId,
      });
      toast.success(`Invitation resent to ${email}.`);
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : "Failed to resend invitation."
      );
    }
  };

  return (
    <div className="space-y-6">
      {/* Members */}
      <Card>
        <CardHeader>
          <div className="flex items-center justify-between">
            <div>
              <CardTitle>Team Members</CardTitle>
              <CardDescription>
                Manage who has access to your organization.
              </CardDescription>
            </div>
            {isAdmin && <InviteMemberDialog organizationId={organizationId} />}
          </div>
        </CardHeader>
        <CardContent>
          {membersLoading && (
            <div className="flex items-center gap-2 text-muted-foreground text-sm">
              <HugeiconsIcon
                className="size-4 animate-spin"
                icon={LoadingIcon}
              />
              Loading members...
            </div>
          )}
          {!membersLoading && (!members || members.length === 0) && (
            <p className="text-muted-foreground text-sm">No members found.</p>
          )}
          {!membersLoading && members && members.length > 0 && (
            <div className="overflow-x-auto">
              <div className="rounded-md border">
                <table className="w-full text-sm">
                  <thead>
                    <tr className="border-b bg-muted/50">
                      <th
                        className="px-4 py-2 text-left font-medium text-muted-foreground"
                        scope="col"
                      >
                        Member
                      </th>
                      <th
                        className="px-4 py-2 text-left font-medium text-muted-foreground"
                        scope="col"
                      >
                        Role
                      </th>
                      <th
                        className="hidden px-4 py-2 text-left font-medium text-muted-foreground sm:table-cell"
                        scope="col"
                      >
                        Joined
                      </th>
                      <th
                        className="px-4 py-2 text-right font-medium text-muted-foreground"
                        scope="col"
                      />
                    </tr>
                  </thead>
                  <tbody>
                    {members.map((member: MemberData) => {
                      const isCurrentUser = member.userId === currentUserId;
                      return (
                        <tr
                          className="border-b last:border-b-0"
                          key={member.id}
                        >
                          <td className="px-4 py-3">
                            <div className="flex items-center gap-3">
                              <Avatar className="size-8">
                                {member.image && (
                                  <AvatarImage
                                    alt={member.name}
                                    src={member.image}
                                  />
                                )}
                                <AvatarFallback className="text-xs">
                                  {getInitials(member.name)}
                                </AvatarFallback>
                              </Avatar>
                              <div className="flex flex-col">
                                <span className="font-medium">
                                  {member.name}
                                  {isCurrentUser && (
                                    <span className="ml-1 text-muted-foreground text-xs">
                                      (you)
                                    </span>
                                  )}
                                </span>
                                <span className="text-muted-foreground text-xs">
                                  {member.email}
                                </span>
                              </div>
                            </div>
                          </td>
                          <td className="px-4 py-3">
                            <ChangeRoleDropdown
                              currentRole={member.role}
                              disabled={
                                !isOwner ||
                                isCurrentUser ||
                                member.role === "owner"
                              }
                              memberId={member.id}
                              memberName={member.name}
                              organizationId={organizationId}
                            />
                          </td>
                          <td className="hidden px-4 py-3 text-muted-foreground sm:table-cell">
                            {formatDate(member.createdAt)}
                          </td>
                          <td className="px-4 py-3 text-right">
                            {isCurrentUser && member.role !== "owner" && (
                              <AlertDialog>
                                <AlertDialogTrigger
                                  render={
                                    <Button
                                      disabled={leaveOrganization.isPending}
                                      variant="outline"
                                    />
                                  }
                                >
                                  {leaveOrganization.isPending ? (
                                    <HugeiconsIcon
                                      className="size-3 animate-spin"
                                      icon={LoadingIcon}
                                    />
                                  ) : (
                                    <HugeiconsIcon
                                      className="size-3"
                                      icon={LogOutIcon}
                                    />
                                  )}
                                  Leave
                                </AlertDialogTrigger>
                                <AlertDialogContent>
                                  <AlertDialogHeader>
                                    <AlertDialogTitle>
                                      Leave Organization?
                                    </AlertDialogTitle>
                                    <AlertDialogDescription>
                                      You will lose access to this organization
                                      immediately. You will need a new
                                      invitation to rejoin.
                                    </AlertDialogDescription>
                                  </AlertDialogHeader>
                                  <AlertDialogFooter>
                                    <AlertDialogCancel>
                                      Cancel
                                    </AlertDialogCancel>
                                    <AlertDialogAction
                                      onClick={() =>
                                        handleLeaveOrganization(member.id)
                                      }
                                    >
                                      Leave Organization
                                    </AlertDialogAction>
                                  </AlertDialogFooter>
                                </AlertDialogContent>
                              </AlertDialog>
                            )}
                            {isAdmin &&
                              !isCurrentUser &&
                              member.role !== "owner" && (
                                <AlertDialog>
                                  <AlertDialogTrigger
                                    render={
                                      <Button
                                        disabled={
                                          removeMember.isPending &&
                                          removeMember.variables
                                            ?.memberIdOrEmail === member.id
                                        }
                                        variant="outline"
                                      />
                                    }
                                  >
                                    {removeMember.isPending &&
                                    removeMember.variables?.memberIdOrEmail ===
                                      member.id ? (
                                      <HugeiconsIcon
                                        className="size-3 animate-spin"
                                        icon={LoadingIcon}
                                      />
                                    ) : (
                                      <HugeiconsIcon
                                        className="size-3"
                                        icon={TrashIcon}
                                      />
                                    )}
                                    Remove
                                  </AlertDialogTrigger>
                                  <AlertDialogContent>
                                    <AlertDialogHeader>
                                      <AlertDialogTitle>
                                        Remove {member.name}?
                                      </AlertDialogTitle>
                                      <AlertDialogDescription>
                                        This will remove {member.name} from the
                                        organization. They will lose access
                                        immediately.
                                      </AlertDialogDescription>
                                    </AlertDialogHeader>
                                    <AlertDialogFooter>
                                      <AlertDialogCancel>
                                        Cancel
                                      </AlertDialogCancel>
                                      <AlertDialogAction
                                        onClick={() =>
                                          handleRemoveMember(member.id)
                                        }
                                        variant="destructive"
                                      >
                                        Remove
                                      </AlertDialogAction>
                                    </AlertDialogFooter>
                                  </AlertDialogContent>
                                </AlertDialog>
                              )}
                          </td>
                        </tr>
                      );
                    })}
                  </tbody>
                </table>
              </div>
            </div>
          )}
        </CardContent>
      </Card>

      {/* Pending Invitations */}
      <Card>
        <CardHeader>
          <CardTitle>Pending Invitations</CardTitle>
          <CardDescription>
            Invitations that have been sent but not yet accepted.
          </CardDescription>
        </CardHeader>
        <CardContent>
          {invitationsLoading && (
            <div className="flex items-center gap-2 text-muted-foreground text-sm">
              <HugeiconsIcon
                className="size-4 animate-spin"
                icon={LoadingIcon}
              />
              Loading invitations...
            </div>
          )}
          {!invitationsLoading && pendingInvitations.length === 0 && (
            <p className="text-muted-foreground text-sm">
              No pending invitations.
            </p>
          )}
          {!invitationsLoading && pendingInvitations.length > 0 && (
            <div className="overflow-x-auto">
              <div className="rounded-md border">
                <table className="w-full text-sm">
                  <thead>
                    <tr className="border-b bg-muted/50">
                      <th
                        className="px-4 py-2 text-left font-medium text-muted-foreground"
                        scope="col"
                      >
                        Email
                      </th>
                      <th
                        className="px-4 py-2 text-left font-medium text-muted-foreground"
                        scope="col"
                      >
                        Role
                      </th>
                      <th
                        className="hidden px-4 py-2 text-left font-medium text-muted-foreground sm:table-cell"
                        scope="col"
                      >
                        Status
                      </th>
                      <th
                        className="hidden px-4 py-2 text-left font-medium text-muted-foreground sm:table-cell"
                        scope="col"
                      >
                        Expires
                      </th>
                      <th
                        className="px-4 py-2 text-right font-medium text-muted-foreground"
                        scope="col"
                      />
                    </tr>
                  </thead>
                  <tbody>
                    {pendingInvitations.map((invitation: InvitationData) => {
                      const isCancelling =
                        cancelInvitation.isPending &&
                        !!cancelInvitation.variables &&
                        typeof cancelInvitation.variables === "object" &&
                        "invitationId" in cancelInvitation.variables &&
                        cancelInvitation.variables.invitationId ===
                          invitation.id;
                      const isExpired =
                        new Date(invitation.expiresAt) < new Date();

                      return (
                        <tr
                          className="border-b last:border-b-0"
                          key={invitation.id}
                        >
                          <td className="px-4 py-3">
                            <div className="flex items-center gap-2">
                              <HugeiconsIcon
                                className="size-4 text-muted-foreground"
                                icon={MailIcon}
                              />
                              {invitation.email}
                            </div>
                          </td>
                          <td className="px-4 py-3">
                            <Badge variant={roleBadgeVariant(invitation.role)}>
                              {invitation.role}
                            </Badge>
                          </td>
                          <td className="hidden px-4 py-3 sm:table-cell">
                            <Badge
                              variant={isExpired ? "destructive" : "outline"}
                            >
                              {isExpired ? "expired" : "pending"}
                            </Badge>
                          </td>
                          <td className="hidden px-4 py-3 text-muted-foreground sm:table-cell">
                            {formatDate(invitation.expiresAt)}
                          </td>
                          <td className="px-4 py-3 text-right">
                            {isAdmin && (
                              <div className="flex items-center justify-end gap-2">
                                {isExpired && (
                                  <Button
                                    disabled={createInvitation.isPending}
                                    onClick={() =>
                                      handleResendInvitation(
                                        invitation.email,
                                        invitation.role
                                      )
                                    }
                                    variant="outline"
                                  >
                                    {createInvitation.isPending ? (
                                      <HugeiconsIcon
                                        className="size-3 animate-spin"
                                        icon={LoadingIcon}
                                      />
                                    ) : (
                                      <HugeiconsIcon
                                        className="size-3"
                                        icon={RefreshIcon}
                                      />
                                    )}
                                    Resend
                                  </Button>
                                )}
                                <Button
                                  disabled={isCancelling}
                                  onClick={() =>
                                    handleCancelInvitation(invitation.id)
                                  }
                                  variant="outline"
                                >
                                  {isCancelling ? (
                                    <HugeiconsIcon
                                      className="size-3 animate-spin"
                                      icon={LoadingIcon}
                                    />
                                  ) : (
                                    <HugeiconsIcon
                                      className="size-3"
                                      icon={TrashIcon}
                                    />
                                  )}
                                  Cancel
                                </Button>
                              </div>
                            )}
                          </td>
                        </tr>
                      );
                    })}
                  </tbody>
                </table>
              </div>
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  );
};

export default TeamMembers;
