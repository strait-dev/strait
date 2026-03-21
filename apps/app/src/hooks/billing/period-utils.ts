/** Convert a "YYYY-MM" period string to from/to date range params. */
export function periodToDateRange(period: string): { from: string; to: string } {
  const [year, month] = period.split("-").map(Number);
  const from = `${year}-${String(month).padStart(2, "0")}-01`;
  const lastDay = new Date(year, month, 0).getDate();
  const to = `${year}-${String(month).padStart(2, "0")}-${String(lastDay).padStart(2, "0")}`;
  return { from, to };
}
