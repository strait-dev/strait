/**
 * Format a date to the Brazilian format.
 * @param date - The date to format.
 * @returns The formatted date.
 */
export function formatDate(date: Date | string | number) {
  return new Intl.DateTimeFormat("pt-BR").format(new Date(date));
}

/**
 * Convert a Unix timestamp to a date.
 * @param secs - The Unix timestamp.
 * @returns The date.
 */
export const toDateTime = (secs: number) => {
  const t = new Date("1970-01-01T00:30:00Z"); // Unix epoch start.
  t.setSeconds(secs);
  return t;
};
