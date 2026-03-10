/**
 * Format options for currency formatting.
 * These are the default options for currency formatting that should be used across all apps.
 * Uses minimumFractionDigits: 0 and maximumFractionDigits: 0 to always hide decimals (e.g., $79 instead of $79.00)
 */
export const formatOptions: Intl.NumberFormatOptions = {
  style: "currency",
  currency: "USD",
  currencyDisplay: "symbol",
  currencySign: "standard",
  minimumFractionDigits: 0,
  maximumFractionDigits: 0,
};

/**
 * Currency formatter for US currency.
 */
export const moneyFormatter = new Intl.NumberFormat("en-US", formatOptions);

/**
 * Format a number to currency format.
 * @param valueOrCurrency - Either the numeric value or the currency code when passing the value as second argument.
 * @param maybeValue - Optional numeric value when currency code is provided as the first argument.
 * @returns The formatted currency string.
 *
 * @example
 * formatCurrency(79) // Returns "$79"
 * formatCurrency(79.99) // Returns "$80" (rounded, no decimals)
 * formatCurrency("USD", 149) // Returns "$149"
 */
export const formatCurrency = (
  valueOrCurrency: number | string,
  maybeValue?: number
): string => {
  if (typeof valueOrCurrency === "number") {
    return moneyFormatter.format(valueOrCurrency);
  }

  const value = maybeValue ?? 0;
  return new Intl.NumberFormat("en-US", {
    ...formatOptions,
    currency: valueOrCurrency,
  }).format(value);
};
