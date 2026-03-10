export type ChartTooltipPayload<T> = {
  payload: T;
  dataKey: keyof T;
  value: number;
};

export type ChartTooltipProps<T> = {
  active?: boolean;
  payload?: ChartTooltipPayload<T>[];
  label?: string;
};

export type BaseChartData = {
  date: string;
  [key: string]: string | number;
};

export type ChartProps<T extends BaseChartData> = {
  data: T[];
  title: string;
  description?: string;
  category: keyof T;
  showYAxis?: boolean;
  showLegend?: boolean;
  showGradient?: boolean;
  colors?: string[];
  className?: string;
};
