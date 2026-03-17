import type { Payload } from "recharts/types/component/DefaultTooltipContent";

type LabelMap = Record<
  string,
  { label: string; color: string; format?: (v: number) => string }
>;

type ChartTooltipProps = {
  active?: boolean;
  payload?: Payload<number, string>[];
  label?: string | number;
  labelMap?: LabelMap;
  valueFormat?: (value: number) => string;
};

const defaultFormat = (v: number) => v.toLocaleString();

export function ChartTooltip({
  active,
  payload,
  label,
  labelMap,
  valueFormat = defaultFormat,
}: ChartTooltipProps) {
  if (!(active && payload?.length)) {
    return null;
  }

  return (
    <div className="rounded-lg border border-border bg-popover px-3 py-2 shadow-md">
      {label != null && (
        <p className="mb-1.5 font-medium text-popover-foreground">{label}</p>
      )}
      <div className="flex flex-col gap-1">
        {payload.map((entry) => {
          const key = entry.dataKey as string;
          const meta = labelMap?.[key];
          const displayLabel = meta?.label ?? key;
          const color = meta?.color ?? entry.color ?? "var(--foreground)";
          const format = meta?.format ?? valueFormat;
          const value = entry.value == null ? "—" : format(entry.value);

          return (
            <div className="flex items-center justify-between gap-4" key={key}>
              <div className="flex items-center gap-2">
                <span
                  className="size-2.5 shrink-0 rounded-full"
                  style={{ backgroundColor: color }}
                />
                <span className="text-muted-foreground capitalize">
                  {displayLabel}
                </span>
              </div>
              <span className="font-medium text-popover-foreground tabular-nums">
                {value}
              </span>
            </div>
          );
        })}
      </div>
    </div>
  );
}
