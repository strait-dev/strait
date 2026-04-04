/**
 * Billing period date range utilities.
 *
 * Converts "YYYY-MM" period strings into from/to date range parameters
 * for the Go backend usage and export API endpoints.
 */

/**
 * Convert a "YYYY-MM" period string to from/to date range params.
 *
 * @param period - A string in "YYYY-MM" format (e.g. "2026-03").
 * @returns An object with `from` (first day) and `to` (last day) in "YYYY-MM-DD" format.
 *
 * @example
 * ```ts
 * periodToDateRange("2026-03") // { from: "2026-03-01", to: "2026-03-31" }
 * ```
 */
export const periodToDateRange = (
  period: string
): { from: string; to: string } => {
  const [year, month] = period.split("-").map(Number);
  const from = `${year}-${String(month).padStart(2, "0")}-01`;
  const lastDay = new Date(year, month, 0).getDate();
  const to = `${year}-${String(month).padStart(2, "0")}-${String(lastDay).padStart(2, "0")}`;
  return { from, to };
};
