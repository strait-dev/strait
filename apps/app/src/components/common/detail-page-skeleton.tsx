import { Shell } from "@strait/ui/components/shell";
import { Skeleton } from "@strait/ui/components/skeleton";

export function DetailPageSkeleton() {
  return (
    <Shell>
      <div className="flex items-center gap-3">
        <Skeleton className="h-5 w-5" />
        <Skeleton className="h-8 w-64" />
        <Skeleton className="ml-auto h-9 w-24" />
      </div>
      <div className="grid gap-4 md:grid-cols-4">
        {Array.from({ length: 4 }).map((_, i) => (
          <div className="rounded-lg border p-4" key={i}>
            <Skeleton className="mb-2 h-4 w-20" />
            <Skeleton className="h-6 w-16" />
          </div>
        ))}
      </div>
      <div className="rounded-lg border">
        <div className="border-b p-4">
          <div className="flex gap-4">
            {Array.from({ length: 3 }).map((_, i) => (
              <Skeleton className="h-8 w-24" key={i} />
            ))}
          </div>
        </div>
        <div className="space-y-3 p-4">
          {Array.from({ length: 6 }).map((_, i) => (
            <Skeleton className="h-4 w-full" key={i} />
          ))}
        </div>
      </div>
    </Shell>
  );
}
