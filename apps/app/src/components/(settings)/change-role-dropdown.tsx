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
} from "@strait/ui/components/alert-dialog";
import { Badge } from "@strait/ui/components/badge";
import { Button } from "@strait/ui/components/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@strait/ui/components/dropdown-menu";
import { Spinner } from "@strait/ui/components/spinner";
import { toast } from "@strait/ui/components/toast";
import { useState } from "react";
import { useUpdateMemberRole } from "@/hooks/auth/use-member";
import { CheckIcon } from "@/lib/icons";

type MemberRole = "owner" | "admin" | "member";

const ROLES: { value: MemberRole; label: string }[] = [
  { value: "owner", label: "Owner" },
  { value: "admin", label: "Admin" },
  { value: "member", label: "Member" },
];

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

interface ChangeRoleDropdownProps {
  currentRole: MemberRole;
  disabled?: boolean;
  memberId: string;
  memberName?: string;
  organizationId: string;
}

const ChangeRoleDropdown = ({
  currentRole,
  disabled,
  memberId,
  memberName,
  organizationId,
}: ChangeRoleDropdownProps) => {
  const updateRole = useUpdateMemberRole();
  const [ownerConfirmOpen, setOwnerConfirmOpen] = useState(false);

  const isUpdating = updateRole.isPending;

  const handleRoleChange = async (role: MemberRole) => {
    if (role === currentRole) {
      return;
    }
    if (role === "owner") {
      setOwnerConfirmOpen(true);
      return;
    }
    try {
      await updateRole.mutateAsync({ memberId, role, organizationId });
      toast.success(`Role updated to ${role}.`);
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : "Failed to update role."
      );
    }
  };

  const handleConfirmOwnerTransfer = async () => {
    try {
      await updateRole.mutateAsync({
        memberId,
        role: "owner",
        organizationId,
      });
      toast.success("Ownership transferred successfully.");
      setOwnerConfirmOpen(false);
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : "Failed to transfer ownership."
      );
    }
  };

  if (disabled) {
    return <Badge variant={roleBadgeVariant(currentRole)}>{currentRole}</Badge>;
  }

  return (
    <>
      <DropdownMenu>
        <DropdownMenuTrigger
          disabled={isUpdating}
          render={<Button size="xs" variant="ghost" />}
        >
          <Badge variant={roleBadgeVariant(currentRole)}>
            {isUpdating ? <Spinner size="xs" /> : currentRole}
          </Badge>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="start">
          {ROLES.map((role) => (
            <DropdownMenuItem
              key={role.value}
              onClick={() => handleRoleChange(role.value)}
            >
              <span className="flex items-center gap-2">
                {role.label}
                {role.value === currentRole && (
                  <HugeiconsIcon className="size-3" icon={CheckIcon} />
                )}
              </span>
            </DropdownMenuItem>
          ))}
        </DropdownMenuContent>
      </DropdownMenu>

      <AlertDialog onOpenChange={setOwnerConfirmOpen} open={ownerConfirmOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Transfer ownership?</AlertDialogTitle>
            <AlertDialogDescription>
              This will make {memberName || "this member"} the owner of the
              organization. You will be demoted to admin.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction
              disabled={isUpdating}
              onClick={handleConfirmOwnerTransfer}
            >
              {isUpdating ? <Spinner size="xs" /> : null}
              Transfer ownership
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </>
  );
};

export default ChangeRoleDropdown;
