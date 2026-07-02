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
import {
  DataGrid,
  DataGridContainer,
  DataGridScrollArea,
  DataGridTable,
} from "@strait/ui/components/data-grid";
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyTitle,
} from "@strait/ui/components/empty";
import { Spinner } from "@strait/ui/components/spinner";
import { toast } from "@strait/ui/components/toast";
import { useQuery } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import { type ColumnDef, getCoreRowModel } from "@tanstack/react-table";
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
import { useAppReactTable } from "@/hooks/use-app-react-table";
import { LogOutIcon, MailIcon, RefreshIcon, TrashIcon } from "@/lib/icons";

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

  const memberColumns: ColumnDef<MemberData>[] = [
    {
      accessorKey: "name",
      header: "Member",
      cell: ({ row }) => {
        const member = row.original;
        const isCurrentUser = member.userId === currentUserId;
        return (
          <div className="flex items-center gap-3">
            <Avatar className="size-8">
              {member.image && (
                <AvatarImage alt={member.name} src={member.image} />
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
        );
      },
    },
    {
      accessorKey: "role",
      header: "Role",
      cell: ({ row }) => {
        const member = row.original;
        const isCurrentUser = member.userId === currentUserId;
        return (
          <ChangeRoleDropdown
            currentRole={member.role}
            disabled={!isOwner || isCurrentUser || member.role === "owner"}
            memberId={member.id}
            memberName={member.name}
            organizationId={organizationId}
          />
        );
      },
    },
    {
      accessorKey: "createdAt",
      header: "Joined",
      cell: ({ row }) => (
        <span className="text-muted-foreground">
          {formatDate(row.original.createdAt)}
        </span>
      ),
    },
    {
      id: "actions",
      cell: ({ row }) => {
        const member = row.original;
        const isCurrentUser = member.userId === currentUserId;
        const isRemoving =
          removeMember.isPending &&
          removeMember.variables?.memberIdOrEmail === member.id;
        return (
          <div className="flex justify-end">
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
                    <Spinner size="xs" />
                  ) : (
                    <HugeiconsIcon className="size-3" icon={LogOutIcon} />
                  )}
                  Leave
                </AlertDialogTrigger>
                <AlertDialogContent>
                  <AlertDialogHeader>
                    <AlertDialogTitle>Leave organization?</AlertDialogTitle>
                    <AlertDialogDescription>
                      You will lose access to this organization immediately. You
                      will need a new invitation to rejoin.
                    </AlertDialogDescription>
                  </AlertDialogHeader>
                  <AlertDialogFooter>
                    <AlertDialogCancel>Cancel</AlertDialogCancel>
                    <AlertDialogAction
                      onClick={() => handleLeaveOrganization(member.id)}
                    >
                      Leave organization
                    </AlertDialogAction>
                  </AlertDialogFooter>
                </AlertDialogContent>
              </AlertDialog>
            )}
            {isAdmin && !isCurrentUser && member.role !== "owner" && (
              <AlertDialog>
                <AlertDialogTrigger
                  render={<Button disabled={isRemoving} variant="outline" />}
                >
                  {isRemoving ? (
                    <Spinner size="xs" />
                  ) : (
                    <HugeiconsIcon className="size-3" icon={TrashIcon} />
                  )}
                  Remove
                </AlertDialogTrigger>
                <AlertDialogContent>
                  <AlertDialogHeader>
                    <AlertDialogTitle>Remove {member.name}?</AlertDialogTitle>
                    <AlertDialogDescription>
                      This will remove {member.name} from the organization. They
                      will lose access immediately.
                    </AlertDialogDescription>
                  </AlertDialogHeader>
                  <AlertDialogFooter>
                    <AlertDialogCancel>Cancel</AlertDialogCancel>
                    <AlertDialogAction
                      onClick={() => handleRemoveMember(member.id)}
                      variant="destructive"
                    >
                      Remove
                    </AlertDialogAction>
                  </AlertDialogFooter>
                </AlertDialogContent>
              </AlertDialog>
            )}
          </div>
        );
      },
      enableSorting: false,
    },
  ];

  const invitationColumns: ColumnDef<InvitationData>[] = [
    {
      accessorKey: "email",
      header: "Email",
      cell: ({ row }) => (
        <div className="flex items-center gap-2">
          <HugeiconsIcon
            className="size-4 text-muted-foreground"
            icon={MailIcon}
          />
          {row.original.email}
        </div>
      ),
    },
    {
      accessorKey: "role",
      header: "Role",
      cell: ({ row }) => (
        <Badge variant={roleBadgeVariant(row.original.role)}>
          {row.original.role}
        </Badge>
      ),
    },
    {
      accessorKey: "status",
      header: "Status",
      cell: ({ row }) => {
        const isExpired = new Date(row.original.expiresAt) < new Date();
        return (
          <Badge variant={isExpired ? "destructive" : "outline"}>
            {isExpired ? "expired" : "pending"}
          </Badge>
        );
      },
    },
    {
      accessorKey: "expiresAt",
      header: "Expires",
      cell: ({ row }) => (
        <span className="text-muted-foreground">
          {formatDate(row.original.expiresAt)}
        </span>
      ),
    },
    {
      id: "actions",
      cell: ({ row }) => {
        const invitation = row.original;
        const isExpired = new Date(invitation.expiresAt) < new Date();
        const isCancelling =
          cancelInvitation.isPending &&
          !!cancelInvitation.variables &&
          typeof cancelInvitation.variables === "object" &&
          "invitationId" in cancelInvitation.variables &&
          cancelInvitation.variables.invitationId === invitation.id;
        return (
          <div className="flex items-center justify-end gap-2">
            {isAdmin && isExpired && (
              <Button
                disabled={createInvitation.isPending}
                onClick={() =>
                  handleResendInvitation(invitation.email, invitation.role)
                }
                variant="outline"
              >
                {createInvitation.isPending ? (
                  <Spinner size="xs" />
                ) : (
                  <HugeiconsIcon className="size-3" icon={RefreshIcon} />
                )}
                Resend
              </Button>
            )}
            {isAdmin && (
              <Button
                disabled={isCancelling}
                onClick={() => handleCancelInvitation(invitation.id)}
                variant="outline"
              >
                {isCancelling ? (
                  <Spinner size="xs" />
                ) : (
                  <HugeiconsIcon className="size-3" icon={TrashIcon} />
                )}
                Cancel
              </Button>
            )}
          </div>
        );
      },
      enableSorting: false,
    },
  ];

  const membersTable = useAppReactTable({
    data: members ?? [],
    columns: memberColumns,
    getCoreRowModel: getCoreRowModel(),
    manualPagination: true,
    getRowId: (row) => row.id,
  });

  const invitationsTable = useAppReactTable({
    data: pendingInvitations,
    columns: invitationColumns,
    getCoreRowModel: getCoreRowModel(),
    manualPagination: true,
    getRowId: (row) => row.id,
  });

  return (
    <div className="space-y-6">
      {/* Members */}
      <Card>
        <CardHeader>
          <div className="flex items-center justify-between">
            <div>
              <CardTitle>Team members</CardTitle>
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
              <Spinner />
              Loading members...
            </div>
          )}
          {!membersLoading && (!members || members.length === 0) && (
            <Empty border={false} className="py-4">
              <EmptyHeader>
                <EmptyTitle>No members found</EmptyTitle>
                <EmptyDescription>
                  Invite teammates to give them access to this organization.
                </EmptyDescription>
              </EmptyHeader>
            </Empty>
          )}
          {!membersLoading && members && members.length > 0 && (
            <DataGrid
              recordCount={members.length}
              table={membersTable}
              tableClassNames={{ base: "min-w-[760px]" }}
            >
              <DataGridContainer>
                <DataGridScrollArea>
                  <DataGridTable />
                </DataGridScrollArea>
              </DataGridContainer>
            </DataGrid>
          )}
        </CardContent>
      </Card>

      {/* Pending Invitations */}
      <Card>
        <CardHeader>
          <CardTitle>Pending invitations</CardTitle>
          <CardDescription>
            Invitations that have been sent but not yet accepted.
          </CardDescription>
        </CardHeader>
        <CardContent>
          {invitationsLoading && (
            <div className="flex items-center gap-2 text-muted-foreground text-sm">
              <Spinner />
              Loading invitations...
            </div>
          )}
          {!invitationsLoading && pendingInvitations.length === 0 && (
            <Empty border={false} className="py-4">
              <EmptyHeader>
                <EmptyTitle>No pending invitations</EmptyTitle>
                <EmptyDescription>
                  Sent invitations will appear here until they are accepted or
                  canceled.
                </EmptyDescription>
              </EmptyHeader>
            </Empty>
          )}
          {!invitationsLoading && pendingInvitations.length > 0 && (
            <DataGrid
              recordCount={pendingInvitations.length}
              table={invitationsTable}
              tableClassNames={{ base: "min-w-[760px]" }}
            >
              <DataGridContainer>
                <DataGridScrollArea>
                  <DataGridTable />
                </DataGridScrollArea>
              </DataGridContainer>
            </DataGrid>
          )}
        </CardContent>
      </Card>
    </div>
  );
};

export default TeamMembers;
