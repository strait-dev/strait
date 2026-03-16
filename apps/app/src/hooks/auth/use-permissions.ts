import { useQuery } from "@tanstack/react-query";
import { membersQueryOptions } from "@/hooks/auth/use-member";

export function useOrganizationRole(organizationId: string, userId: string) {
  const { data: members } = useQuery(membersQueryOptions({ organizationId }));
  const currentMember = members?.find((m) => m.userId === userId);
  return {
    role: currentMember?.role ?? null,
    isOwner: currentMember?.role === "owner",
    isAdmin: currentMember?.role === "admin" || currentMember?.role === "owner",
    isMember: !!currentMember,
  };
}
