/** Format a micro-USD integer (1 USD = 1,000,000) as a dollar string. */
export function formatMicroUsd(microUsd: number): string {
  return `$${(microUsd / 1_000_000).toFixed(2)}`;
}
