/**
 * Organization-like type that has currencyCode field.
 * This is flexible enough to accept both direct Organization types
 * and API response types.
 */
type OrganizationWithCurrency =
  | {
      currencyCode?: string | null;
    }
  | null
  | undefined;

/**
 * Currency symbol map for supported currencies
 */
const CURRENCY_SYMBOLS: Record<string, string> = {
  USD: "$",
  EUR: "€",
  GBP: "£",
  JPY: "¥",
  CAD: "C$",
  AUD: "A$",
  CHF: "CHF",
  CNY: "¥",
  SEK: "kr",
  NZD: "NZ$",
  MXN: "$",
  SGD: "S$",
  HKD: "HK$",
  NOK: "kr",
  DKK: "kr",
  PLN: "zł",
  CZK: "Kč",
  HUF: "Ft",
  BRL: "R$",
  INR: "₹",
  KRW: "₩",
  ZAR: "R",
  TRY: "₺",
  RUB: "₽",
  ILS: "₪",
};

/**
 * Get currency symbol for a given currency code
 */
function getCurrencySymbol(currencyCode: string): string {
  return CURRENCY_SYMBOLS[currencyCode] || currencyCode;
}

/**
 * Format currency placeholder for input fields
 * @param currencyCode - The currency code (e.g., 'USD', 'BRL', 'EUR')
 * @returns Formatted placeholder string (e.g., '$0', 'R$0', '€0')
 */
function getCurrencyPlaceholder(currencyCode: string): string {
  const symbol = getCurrencySymbol(currencyCode);
  return `${symbol}0`;
}

/**
 * Get format options for NumberInput based on organization currency
 * @param organization - The organization object containing currency_code
 * @returns Intl.NumberFormat options for currency formatting
 */
export function getCurrencyFormatOptions(
  organization?: OrganizationWithCurrency
): {
  style: "currency";
  currency: string;
  minimumFractionDigits: number;
  maximumFractionDigits: number;
} {
  const currencyCode = organization?.currencyCode || "USD";

  return {
    style: "currency" as const,
    currency: currencyCode,
    minimumFractionDigits: 0,
    maximumFractionDigits: 0,
  };
}

/**
 * Get currency placeholder based on organization currency
 * @param organization - The organization object containing currency_code
 * @returns Formatted placeholder string for the organization's currency
 */
export function getOrganizationCurrencyPlaceholder(
  organization?: OrganizationWithCurrency
): string {
  const currencyCode = organization?.currencyCode || "USD";
  return getCurrencyPlaceholder(currencyCode);
}
