"use client";
import {
  Bar,
  CartesianGrid,
  Cell,
  Line,
  Pie,
  BarChart as RechartsBarChart,
  LineChart as RechartsLineChart,
  PieChart as RechartsPieChart,
  ResponsiveContainer,
  XAxis,
  YAxis,
} from "recharts";

const BAR_RADIUS = 4;

type BaseChartProps = {
  data: any[];
  index: string;
  colors?: string[];
  valueFormatter?: (value: number) => string;
};

type LineChartProps = BaseChartProps & {
  categories: string[];
};

type BarChartProps = BaseChartProps & {
  categories: string[];
  layout?: "vertical" | "horizontal";
};

type PieChartProps = BaseChartProps & {
  category: string;
};

export const LineChart = ({
  data,
  categories,
  index,
  colors = ["hsl(var(--primary))"],
  valueFormatter = (value) => value.toString(),
}: LineChartProps) => (
  <ResponsiveContainer height={300} width="100%">
    <RechartsLineChart data={data}>
      <CartesianGrid vertical={false} />
      <XAxis
        axisLine={false}
        dataKey={index}
        tickLine={false}
        tickMargin={10}
      />
      <YAxis
        axisLine={false}
        tickFormatter={valueFormatter}
        tickLine={false}
        tickMargin={10}
      />
      {categories.map((category, i) => (
        <Line
          dataKey={category}
          dot={false}
          key={category}
          stroke={colors[i % colors.length]}
          strokeWidth={2}
          type="monotone"
        />
      ))}
    </RechartsLineChart>
  </ResponsiveContainer>
);

export const BarChart = ({
  data,
  categories,
  index,
  colors = ["hsl(var(--primary))"],
  valueFormatter = (value) => value.toString(),
  layout = "horizontal",
}: BarChartProps) => (
  <ResponsiveContainer height={300} width="100%">
    <RechartsBarChart
      data={data}
      layout={layout}
      margin={{ top: 5, right: 5, bottom: 5, left: 5 }}
    >
      <CartesianGrid vertical={false} />
      <XAxis
        axisLine={false}
        dataKey={layout === "horizontal" ? index : undefined}
        tickLine={false}
        tickMargin={10}
        type={layout === "horizontal" ? "category" : "number"}
      />
      <YAxis
        axisLine={false}
        dataKey={layout === "vertical" ? index : undefined}
        tickFormatter={valueFormatter}
        tickLine={false}
        tickMargin={10}
        type={layout === "vertical" ? "category" : "number"}
      />
      {categories.map((category, i) => (
        <Bar
          dataKey={category}
          fill={colors[i % colors.length]}
          key={category}
          radius={[BAR_RADIUS, BAR_RADIUS, 0, 0]}
        />
      ))}
    </RechartsBarChart>
  </ResponsiveContainer>
);

export const PieChart = ({
  data,
  category,
  index,
  colors = ["hsl(var(--primary))"],
}: PieChartProps) => (
  <div className="aspect-square w-full">
    <ResponsiveContainer height="100%" width="100%">
      <RechartsPieChart>
        <Pie
          cx="50%"
          cy="50%"
          data={data}
          dataKey={category}
          fill="#8884d8"
          labelLine={false}
          outerRadius={80}
        >
          {data.map((entry) => (
            <Cell
              fill={colors[data.indexOf(entry) % colors.length]}
              key={`${entry[index]}-${entry[category]}`}
              stroke="hsl(var(--background))"
              strokeWidth={2}
            />
          ))}
        </Pie>
      </RechartsPieChart>
    </ResponsiveContainer>
  </div>
);

export const MapChart = () => {
  // TODO: Implement map chart
  return (
    <div className="aspect-square w-full">
      <div className="flex h-full items-center justify-center text-muted-foreground">
        Map chart component not implemented yet
      </div>
    </div>
  );
};
