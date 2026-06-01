import { Shell } from "@strait/ui/components/shell";
import { Skeleton } from "@strait/ui/components/skeleton";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@strait/ui/components/table";

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
    <Table size="lg" variant="bordered">
      <TableHeader>
        <TableRow>
          {HEADER_KEYS.map((key) => (
            <TableHead key={key}>
              <Skeleton className="h-4 w-24" />
            </TableHead>
          ))}
        </TableRow>
      </TableHeader>
      <TableBody>
        {ROW_KEYS.map((rowKey) => (
          <TableRow key={rowKey}>
            {CELL_KEYS.map((cellKey) => (
              <TableCell key={cellKey}>
                <Skeleton className="h-4 w-24" />
              </TableCell>
            ))}
          </TableRow>
        ))}
      </TableBody>
    </Table>
  </Shell>
);

export default TablePageSkeleton;
