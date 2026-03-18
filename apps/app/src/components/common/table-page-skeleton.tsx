import { Shell } from "@strait/ui/components/shell";
import { Skeleton } from "@strait/ui/components/skeleton";

export function TablePageSkeleton() {
  return (
    <Shell>
      <div className="flex items-center justify-between">
        <Skeleton className="h-8 w-48" />
        <Skeleton className="h-9 w-32" />
      </div>
      <div className="flex items-center gap-2">
        <Skeleton className="h-9 w-64" />
        <Skeleton className="h-9 w-24" />
      </div>
      <div className="rounded-md border">
        <div className="border-b px-4 py-3">
          <div className="flex gap-8">
            {Array.from({ length: 5 }).map((_, i) => (
              <Skeleton className="h-4 w-24" key={i} />
            ))}
          </div>
        </div>
        {Array.from({ length: 8 }).map((_, i) => (
          <div className="flex gap-8 border-b px-4 py-3 last:border-b-0" key={i}>
            {Array.from({ length: 5 }).map((_, j) => (
              <Skeleton className="h-4 w-24" key={j} />
            ))}
          </div>
        ))}
      </div>
    </Shell>
  );
}
