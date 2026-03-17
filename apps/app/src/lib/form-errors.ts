/**
 * Extracts human-readable error messages from TanStack Form validation errors.
 *
 * TanStack Form + Zod can return errors as strings, objects with a `message`
 * property, or other shapes. Using `.join()` directly on these produces
 * `[object Object]`. This helper normalizes them to a single display string.
 */
export function formatFieldErrors(
  errors: Array<string | { message: string } | unknown>
): string {
  return errors
    .map((e) => {
      if (typeof e === "string") {
        return e;
      }
      if (e && typeof e === "object" && "message" in e) {
        return (e as { message: string }).message;
      }
      return String(e);
    })
    .join(", ");
}
