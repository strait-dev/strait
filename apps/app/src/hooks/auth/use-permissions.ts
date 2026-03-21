import { useQuery } from "@tanstack/react-query";
import type { MemberData } from "@/hooks/auth/use-member";
import { membersQueryOptions } from "@/hooks/auth/use-member";

/** Returns the current user's role and permission flags within an organization. */
export const useOrganizationRole = (organizationId: string, userId: string) => {
  const { data: members } = useQuery(membersQueryOptions({ organizationId }));
  const currentMember = members?.find((m: MemberData) => m.userId === userId);
  return {
    role: currentMember?.role ?? null,
    isOwner: currentMember?.role === "owner",
    isAdmin: currentMember?.role === "admin" || currentMember?.role === "owner",
    isMember: !!currentMember,
  };
};
