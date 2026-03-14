import type { BadgeProps } from "@strait/ui/components/badge";
import { Badge } from "@strait/ui/components/badge";
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@strait/ui/components/card";
import { cn } from "@strait/ui/utils/index";
import type { ReactNode } from "react";

// ---------------------------------------------------------------------------
// DetailSection – a Card-based section with title, optional icon and action
// ---------------------------------------------------------------------------

type DetailSectionProps = {
  title: string;
  icon?: ReactNode;
  action?: ReactNode;
  children: ReactNode;
  className?: string;
};

export function DetailSection({
  title,
  icon,
  action,
  children,
  className,
}: DetailSectionProps) {
  return (
    <Card className={cn("shadow-sm", className)}>
      <CardHeader className="flex-row items-center justify-between border-b">
        <div className="flex items-center gap-2">
          {icon ? (
            <div className="flex size-8 shrink-0 items-center justify-center rounded-md bg-primary/10 text-primary">
              {icon}
            </div>
          ) : null}
          <CardTitle>{title}</CardTitle>
        </div>
        {action ? <div>{action}</div> : null}
      </CardHeader>
      <CardContent className="grid gap-3">{children}</CardContent>
    </Card>
  );
}

// ---------------------------------------------------------------------------
// DetailField – a clean label ↔ value pair replacing the old InfoRow
// ---------------------------------------------------------------------------

type DetailFieldProps = {
  label: string;
  value: ReactNode;
  className?: string;
};

export function DetailField({ label, value, className }: DetailFieldProps) {
  return (
    <div className={cn("flex items-center justify-between gap-4", className)}>
      <span className="shrink-0 text-muted-foreground text-sm">{label}</span>
      <span className="text-right font-medium text-sm">{value || "—"}</span>
    </div>
  );
}

// ---------------------------------------------------------------------------
// StatCard – a polished single-metric card with optional icon
// ---------------------------------------------------------------------------

type StatCardProps = {
  label: string;
  value: string;
  icon?: ReactNode;
  description?: string;
  className?: string;
};

export function StatCard({
  label,
  value,
  icon,
  description,
  className,
}: StatCardProps) {
  return (
    <Card className={cn("shadow-sm", className)}>
      <CardContent className="flex items-start gap-3 py-0">
        {icon ? (
          <div className="flex size-10 shrink-0 items-center justify-center rounded-lg bg-primary/10 text-primary">
            {icon}
          </div>
        ) : null}
        <div className="min-w-0 flex-1">
          <p className="truncate text-muted-foreground text-sm">{label}</p>
          <p className="truncate font-semibold text-xl tracking-tight">
            {value}
          </p>
          {description ? (
            <p className="truncate text-muted-foreground text-xs">
              {description}
            </p>
          ) : null}
        </div>
      </CardContent>
    </Card>
  );
}

// ---------------------------------------------------------------------------
// DetailGrid – a responsive 2-column grid for detail fields
// ---------------------------------------------------------------------------

type DetailGridProps = {
  children: ReactNode;
  columns?: 1 | 2 | 3 | 4;
  className?: string;
};

export function DetailGrid({
  children,
  columns = 2,
  className,
}: DetailGridProps) {
  const colsMap = {
    1: "grid-cols-1",
    2: "grid-cols-1 md:grid-cols-2",
    3: "grid-cols-1 md:grid-cols-2 lg:grid-cols-3",
    4: "grid-cols-1 md:grid-cols-2 lg:grid-cols-4",
  } as const;

  return (
    <div className={cn("grid gap-4", colsMap[columns], className)}>
      {children}
    </div>
  );
}

// ---------------------------------------------------------------------------
// StatusField – a detail field that renders a Badge for the value
// ---------------------------------------------------------------------------

type StatusFieldProps = {
  label: string;
  status: string;
  variant?: BadgeProps["variant"];
  className?: string;
};

export function StatusField({
  label,
  status,
  variant = "secondary-light",
  className,
}: StatusFieldProps) {
  return (
    <div className={cn("flex items-center justify-between gap-4", className)}>
      <span className="shrink-0 text-muted-foreground text-sm">{label}</span>
      <Badge variant={variant}>{status}</Badge>
    </div>
  );
}

// ---------------------------------------------------------------------------
// EmptyTabContent – consistent empty state for tab content
// ---------------------------------------------------------------------------

type EmptyTabContentProps = {
  title: string;
  description: string;
  icon?: ReactNode;
  action?: ReactNode;
};

export function EmptyTabContent({
  title,
  description,
  icon,
  action,
}: EmptyTabContentProps) {
  return (
    <div className="flex flex-col items-center justify-center gap-3 py-16 text-center">
      {icon ? (
        <div className="flex size-12 items-center justify-center rounded-full bg-muted text-muted-foreground">
          {icon}
        </div>
      ) : null}
      <div className="space-y-1">
        <h3 className="font-semibold text-base">{title}</h3>
        <p className="max-w-sm text-muted-foreground text-sm">{description}</p>
      </div>
      {action ? <div className="mt-2">{action}</div> : null}
    </div>
  );
}

// ---------------------------------------------------------------------------
// EntityTable – a styled table wrapper for related items in view pages
// ---------------------------------------------------------------------------

type Column<T> = {
  header: string;
  accessorKey?: keyof T;
  cell?: (row: T) => ReactNode;
  align?: "left" | "right" | "center";
  className?: string;
};

type EntityTableProps<T> = {
  columns: Column<T>[];
  data: T[];
  getRowKey: (row: T) => string;
  onRowClick?: (row: T) => void;
};

function renderCellContent<T>(col: Column<T>, row: T): ReactNode {
  if (col.cell) {
    return col.cell(row);
  }

  if (col.accessorKey) {
    return String(
      (row as Record<string, unknown>)[col.accessorKey as string] ?? "—"
    );
  }

  return "—";
}

export function EntityTable<T>({
  columns,
  data,
  getRowKey,
  onRowClick,
}: EntityTableProps<T>) {
  return (
    <Card className="shadow-sm">
      <div className="overflow-x-auto">
        <table className="w-full">
          <thead>
            <tr className="border-b bg-muted/30">
              {columns.map((col) => (
                <th
                  className={cn(
                    "px-4 py-3 font-medium text-muted-foreground text-sm",
                    col.align === "right" ? "text-right" : "text-left",
                    col.className
                  )}
                  key={col.header}
                >
                  {col.header}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {data.map((row) => (
              <tr
                className={cn(
                  "border-b transition-colors last:border-b-0 hover:bg-muted/30",
                  onRowClick ? "cursor-pointer" : ""
                )}
                key={getRowKey(row)}
                onClick={onRowClick ? () => onRowClick(row) : undefined}
              >
                {columns.map((col) => (
                  <td
                    className={cn(
                      "px-4 py-3 text-sm",
                      col.align === "right" ? "text-right" : "text-left",
                      col.className
                    )}
                    key={col.header}
                  >
                    {renderCellContent(col, row)}
                  </td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </Card>
  );
}
