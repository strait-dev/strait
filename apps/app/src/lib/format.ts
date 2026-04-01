const currencyFormatOptions: Intl.NumberFormatOptions = {
  style: "currency",
  currency: "USD",
  currencyDisplay: "symbol",
  currencySign: "standard",
  minimumFractionDigits: 0,
  maximumFractionDigits: 0,
};

const moneyFormatter = new Intl.NumberFormat("en-US", currencyFormatOptions);

/** Format a number as currency (e.g., 79 -> "$79"). */
export function formatCurrency(
  valueOrCurrency: number | string,
  maybeValue?: number
): string {
  if (typeof valueOrCurrency === "number") {
    return moneyFormatter.format(valueOrCurrency);
  }
  return new Intl.NumberFormat("en-US", {
    ...currencyFormatOptions,
    currency: valueOrCurrency,
  }).format(maybeValue ?? 0);
}

/** Format a micro-USD integer (1 USD = 1,000,000) as a dollar string. */
export function formatMicroUsd(microUsd: number): string {
  return `$${(microUsd / 1_000_000).toFixed(2)}`;
}

/** Capitalize the first letter of a string. */
export function capitalize(str: string): string {
  return str.charAt(0).toUpperCase() + str.slice(1);
}

/** Format a duration between two timestamps as a human-readable string. */
export function formatDuration(
  start: string | null,
  end: string | null
): string {
  if (!start) {
    return "-";
  }
  const s = new Date(start).getTime();
  const e = end ? new Date(end).getTime() : Date.now();
  const ms = e - s;
  if (ms < 1000) {
    return `${ms}ms`;
  }
  if (ms < 60_000) {
    return `${(ms / 1000).toFixed(1)}s`;
  }
  const mins = Math.floor(ms / 60_000);
  const secs = Math.round((ms % 60_000) / 1000);
  return `${mins}m ${secs}s`;
}
