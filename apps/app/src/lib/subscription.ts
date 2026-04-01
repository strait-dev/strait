import type { Polar } from "@polar-sh/sdk";
import { createServerFn } from "@tanstack/react-start";

/**
 * Lazily initialized Polar SDK client singleton.
 *
 * Uses a dynamic import for `@polar-sh/sdk` because that package depends
 * on `tsyringe`, which checks for `Reflect.getMetadata` at module
 * evaluation time. A top-level import would crash the Cloudflare Worker
 * before any request handling code runs.
 *
 * Initialization is also deferred because Cloudflare Workers only
 * populate `process.env` during request handling, not at module load time.
 */
let _polarClient: Polar | null = null;

async function getPolarClient(): Promise<Polar> {
  if (!_polarClient) {
    const { Polar: PolarClient } = await import("@polar-sh/sdk");
    _polarClient = new PolarClient({
      accessToken: process.env.POLAR_ACCESS_TOKEN ?? "",
      server:
        (process.env.POLAR_SERVER as "sandbox" | "production") ?? "production",
    });
  }
  return _polarClient;
}

type CustomerPortalResponse = {
  url: string | null;
  error: string | null;
};

/**
 * Server function to get customer portal URL using email lookup.
 * This works around the limitation where customers don't have externalId set.
 */
export const getCustomerPortalUrlServerFn = createServerFn({
  method: "GET",
}).handler(async ({ context }): Promise<CustomerPortalResponse> => {
  const ctx = context as { session?: { user: { email: string } } } | undefined;
  const session = ctx?.session;

  if (!session) {
    return {
      url: null,
      error: "Session not found",
    };
  }

  try {
    const client = await getPolarClient();

    // Look up the Polar customer by email
    const { result: customersResult } = await client.customers.list({
      email: session.user.email,
      limit: 1,
    });

    const customers = customersResult.items;

    if (!Array.isArray(customers) || customers.length === 0) {
      return {
        url: null,
        error: "Customer not found",
      };
    }

    const polarCustomerId = customers[0].id;

    // Create a customer session using the Polar customer ID
    const customerSession = await client.customerSessions.create({
      customerId: polarCustomerId,
    });

    return {
      url: customerSession.customerPortalUrl,
      error: null,
    };
  } catch (error) {
    console.error("Failed to create customer portal session:", error);
    return {
      url: null,
      error: "Failed to create customer portal session",
    };
  }
});
