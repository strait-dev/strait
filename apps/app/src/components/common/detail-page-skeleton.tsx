import { Card, CardContent, CardHeader } from "@strait/ui/components/card";
import { Shell } from "@strait/ui/components/shell";
import { Skeleton } from "@strait/ui/components/skeleton";

const METRIC_KEYS = ["metric-0", "metric-1", "metric-2", "metric-3"];
const TAB_KEYS = ["tab-0", "tab-1", "tab-2"];
const ROW_KEYS = ["row-0", "row-1", "row-2", "row-3", "row-4", "row-5"];

const DetailPageSkeleton = () => (
  <Shell>
    <div className="flex items-center gap-3">
      <Skeleton className="size-5" />
      <Skeleton className="h-8 w-64" />
      <Skeleton className="ml-auto h-9 w-24" />
    </div>
    <div className="grid gap-4 md:grid-cols-4">
      {METRIC_KEYS.map((key) => (
        <Card key={key}>
          <CardContent>
            <Skeleton className="mb-2 h-4 w-20" />
            <Skeleton className="h-6 w-16" />
          </CardContent>
        </Card>
      ))}
    </div>
    <Card>
      <CardHeader>
        <div className="flex gap-4">
          {TAB_KEYS.map((key) => (
            <Skeleton className="h-8 w-24" key={key} />
          ))}
        </div>
      </CardHeader>
      <CardContent className="space-y-3">
        {ROW_KEYS.map((key) => (
          <Skeleton className="h-4 w-full" key={key} />
        ))}
      </CardContent>
    </Card>
  </Shell>
);

export default DetailPageSkeleton;
