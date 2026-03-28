import { Polar } from "@polar-sh/sdk";
import { createServerFn } from "@tanstack/react-start";

const disablePolarBilling = process.env.DISABLE_POLAR_BILLING === "true";

const polarClient =
  !disablePolarBilling && process.env.POLAR_ACCESS_TOKEN
    ? new Polar({
        accessToken: process.env.POLAR_ACCESS_TOKEN,
        server:
          (process.env.POLAR_SERVER as "sandbox" | "production") ??
          "production",
      })
    : null;

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

  if (!polarClient) {
    return {
      url: null,
      error: "Billing portal unavailable",
    };
  }

  try {
    // Look up the Polar customer by email
    const { result: customersResult } = await polarClient.customers.list({
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
    const customerSession = await polarClient.customerSessions.create({
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
