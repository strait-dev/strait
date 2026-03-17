import * as z from "zod";

/**
 * Validates that a redirect URL is a safe relative path.
 * Prevents open redirect attacks by rejecting absolute URLs,
 * protocol-relative URLs, and other dangerous patterns.
 */
const safeRedirect = z
  .string()
  .optional()
  .transform((val) => {
    if (!val) {
      return undefined;
    }
    // Only allow relative paths starting with /
    // Reject absolute URLs, protocol-relative, and javascript: URIs
    if (
      !val.startsWith("/") ||
      val.startsWith("//") ||
      val.startsWith("/\\") ||
      val.toLowerCase().includes("://")
    ) {
      return undefined;
    }
    return val;
  })
  .catch(undefined);

export const authSearchSchema = z.object({
  redirect: safeRedirect,
  error: z.string().optional().catch(undefined),
});
