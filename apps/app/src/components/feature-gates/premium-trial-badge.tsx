import { SparklesIcon } from "@hugeicons/core-free-icons";
import { HugeiconsIcon } from "@hugeicons/react";
import { Badge } from "@strait/ui/components/badge.tsx";
import { useSuspenseQuery } from "@tanstack/react-query";
import { subscriptionStateQueryOptions } from "@/hooks/subscription/use-subscription.ts";

/**
 * Premium Trial Feature Badge
 *
 * Simplified version for quick feature highlighting
 */
export const PremiumFeatureBadge = ({ className }: { className?: string }) => {
  const { data } = useSuspenseQuery(subscriptionStateQueryOptions());
  const { isTrialing } = data;

  if (!isTrialing) {
    return null;
  }

  return (
    <Badge
      className={`inline-flex items-center gap-1 border-purple-200/60 bg-gradient-to-r from-purple-50 to-pink-50 px-2 py-0.5 text-purple-800 text-xs hover:from-purple-100 hover:to-pink-100 dark:border-purple-800/30 dark:from-purple-950/30 dark:to-pink-950/30 dark:text-purple-200 ${className}
      `}
      title="Premium trial feature"
      variant="secondary"
    >
      <HugeiconsIcon className="h-3 w-3" icon={SparklesIcon} />
      <span className="font-medium">Premium</span>
    </Badge>
  );
};
