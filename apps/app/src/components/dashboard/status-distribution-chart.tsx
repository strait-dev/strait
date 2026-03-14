import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@strait/ui/components/card.tsx";
// biome-ignore lint/suspicious/noDeprecatedImports: recharts Cell is the only API for per-slice styling
import { Cell, Pie, PieChart, ResponsiveContainer } from "recharts";

const MOCK_DATA = [
  { name: "Completed", value: 342, color: "var(--color-chart-1)" },
  { name: "Executing", value: 28, color: "var(--color-chart-3)" },
  { name: "Queued", value: 15, color: "var(--color-chart-2)" },
  { name: "Failed", value: 12, color: "var(--color-chart-4)" },
];

export function StatusDistributionChart() {
  const total = MOCK_DATA.reduce((sum, d) => sum + d.value, 0);

  return (
    <Card>
      <CardHeader className="pb-2">
        <CardTitle className="font-medium text-sm">
          Status Distribution
        </CardTitle>
      </CardHeader>
      <CardContent>
        <div className="flex items-center gap-6">
          <div className="h-[180px] w-[180px] shrink-0">
            <ResponsiveContainer height="100%" width="100%">
              <PieChart>
                <Pie
                  cx="50%"
                  cy="50%"
                  data={MOCK_DATA}
                  dataKey="value"
                  innerRadius={55}
                  outerRadius={80}
                  paddingAngle={2}
                  strokeWidth={0}
                >
                  {MOCK_DATA.map((entry) => (
                    <Cell fill={entry.color} key={entry.name} />
                  ))}
                </Pie>
              </PieChart>
            </ResponsiveContainer>
          </div>
          <div className="flex flex-col gap-3">
            {MOCK_DATA.map((entry) => {
              const pct = ((entry.value / total) * 100).toFixed(1);
              return (
                <div className="flex items-center gap-2" key={entry.name}>
                  <span
                    className="h-2.5 w-2.5 shrink-0 rounded-full"
                    style={{ backgroundColor: entry.color }}
                  />
                  <span className="text-muted-foreground text-sm">
                    {entry.name}
                  </span>
                  <span className="ml-auto font-medium font-mono text-sm">
                    {entry.value}
                  </span>
                  <span className="text-muted-foreground text-xs">
                    ({pct}%)
                  </span>
                </div>
              );
            })}
          </div>
        </div>
      </CardContent>
    </Card>
  );
}
