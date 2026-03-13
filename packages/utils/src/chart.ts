import { moneyFormatter } from "./money.ts";

/**
 * Format a percentage to the Brazilian format.
 * @param value - The percentage to format.
 * @returns The formatted percentage.
 */
export const formatPercentage = (value: number): string =>
  `${value.toFixed(1)}%`;

/**
 * Format a chart change.
 * @param payload - The payload.
 * @param percentageChange - The percentage change.
 * @param absoluteChange - The absolute change.
 * @param isCurrency - Whether the change is in currency.
 * @returns The formatted chart change.
 */
type FormatChartChangeOptions = {
  payload: any;
  percentageChange: number;
  absoluteChange: number;
  isCurrency?: boolean;
};

/**
 * Format a chart change.
 * @param payload - The payload.
 * @param percentageChange - The percentage change.
 * @param absoluteChange - The absolute change.
 * @param isCurrency - Whether the change is in currency.
 * @returns The formatted chart change.
 */
export const formatChartChange = ({
  payload,
  percentageChange,
  absoluteChange,
  isCurrency = true,
}: FormatChartChangeOptions): string => {
  if (!payload || Number.isNaN(percentageChange)) {
    return "--";
  }

  const formattedPercentage = `${
    percentageChange > 0 ? "+" : ""
  }${percentageChange.toFixed(1)}%`;

  const formattedAbsolute = isCurrency
    ? `${absoluteChange >= 0 ? "+" : "-"}${moneyFormatter.format(
        Math.abs(absoluteChange)
      )}`
    : `${absoluteChange >= 0 ? "+" : "-"}${Math.abs(absoluteChange).toFixed(1)}%`;

  return `${formattedPercentage} (${formattedAbsolute})`;
};
