import { useQuery } from "@tanstack/react-query";
import { orgUsageQueryOptions } from "@/hooks/billing/use-org-usage";

/** Returns the current org's plan slug, defaulting to "free". */
export function useCurrentPlan(): string {
  const { data } = useQuery(orgUsageQueryOptions());
  return data?.plan ?? "free";
}
