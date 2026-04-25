import { Shell } from "@strait/ui/components/shell";
import { Skeleton } from "@strait/ui/components/skeleton";

const HEADER_KEYS = ["hdr-0", "hdr-1", "hdr-2", "hdr-3", "hdr-4"];
const ROW_KEYS = [
  "row-0",
  "row-1",
  "row-2",
  "row-3",
  "row-4",
  "row-5",
  "row-6",
  "row-7",
];
const CELL_KEYS = ["cell-0", "cell-1", "cell-2", "cell-3", "cell-4"];

const TablePageSkeleton = () => (
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
          {HEADER_KEYS.map((key) => (
            <Skeleton className="h-4 w-24" key={key} />
          ))}
        </div>
      </div>
      {ROW_KEYS.map((rowKey) => (
        <div
          className="flex gap-8 border-b px-4 py-3 last:border-b-0"
          key={rowKey}
        >
          {CELL_KEYS.map((cellKey) => (
            <Skeleton className="h-4 w-24" key={cellKey} />
          ))}
        </div>
      ))}
    </div>
  </Shell>
);

export default TablePageSkeleton;
