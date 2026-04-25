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
      return;
    }
    // Only allow relative paths starting with /
    // Reject absolute URLs, protocol-relative, and javascript: URIs
    if (
      !val.startsWith("/") ||
      val.startsWith("//") ||
      val.startsWith("/\\") ||
      val.toLowerCase().includes("://")
    ) {
      return;
    }
    return val;
  })
  .catch(undefined);

export const authSearchSchema = z.object({
  redirect: safeRedirect,
  error: z.string().optional().catch(undefined),
  utm_source: z.string().optional().catch(undefined),
  utm_medium: z.string().optional().catch(undefined),
  utm_campaign: z.string().optional().catch(undefined),
  utm_term: z.string().optional().catch(undefined),
  utm_content: z.string().optional().catch(undefined),
  ref: z.string().optional().catch(undefined),
});
