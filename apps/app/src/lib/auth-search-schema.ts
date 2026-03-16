import * as z from "zod";

export const authSearchSchema = z.object({
  redirect: z.string().optional().catch(undefined),
  error: z.string().optional().catch(undefined),
});
