import { auth } from "./auth.server";

/**
 * Better Auth handler for the API route.
 * Processes all /api/auth/* requests.
 */
export const handler = (request: Request) => auth.handler(request);
