import type { ComponentProps } from "react";
import { ResponsiveContainer } from "recharts";

type ResponsiveChartContainerProps = ComponentProps<typeof ResponsiveContainer>;

const DEFAULT_INITIAL_DIMENSION = { width: 1, height: 1 };

/**
 * Dashboard chart container with a positive first render size.
 *
 * Recharts initializes responsive containers at -1 x -1 before ResizeObserver
 * reports the real dimensions, which creates noisy warnings during SSR/dev
 * hydration. A 1 x 1 initial size keeps the first render valid while the
 * measured chart size still takes over immediately after mount.
 */
const ResponsiveChartContainer = ({
  initialDimension = DEFAULT_INITIAL_DIMENSION,
  minHeight = 1,
  minWidth = 1,
  ...props
}: ResponsiveChartContainerProps) => (
  <ResponsiveContainer
    initialDimension={initialDimension}
    minHeight={minHeight}
    minWidth={minWidth}
    {...props}
  />
);

export default ResponsiveChartContainer;
