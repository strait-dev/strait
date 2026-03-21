import { Shell } from "@strait/ui/components/shell";
import { Skeleton } from "@strait/ui/components/skeleton";

const METRIC_KEYS = ["metric-0", "metric-1", "metric-2", "metric-3"];
const TAB_KEYS = ["tab-0", "tab-1", "tab-2"];
const ROW_KEYS = ["row-0", "row-1", "row-2", "row-3", "row-4", "row-5"];

const DetailPageSkeleton = () => {
  return (
    <Shell>
      <div className="flex items-center gap-3">
        <Skeleton className="h-5 w-5" />
        <Skeleton className="h-8 w-64" />
        <Skeleton className="ml-auto h-9 w-24" />
      </div>
      <div className="grid gap-4 md:grid-cols-4">
        {METRIC_KEYS.map((key) => (
          <div className="rounded-lg border p-4" key={key}>
            <Skeleton className="mb-2 h-4 w-20" />
            <Skeleton className="h-6 w-16" />
          </div>
        ))}
      </div>
      <div className="rounded-lg border">
        <div className="border-b p-4">
          <div className="flex gap-4">
            {TAB_KEYS.map((key) => (
              <Skeleton className="h-8 w-24" key={key} />
            ))}
          </div>
        </div>
        <div className="space-y-3 p-4">
          {ROW_KEYS.map((key) => (
            <Skeleton className="h-4 w-full" key={key} />
          ))}
        </div>
      </div>
    </Shell>
  );
};

export default DetailPageSkeleton;
