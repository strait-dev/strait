import { createServerFn } from "@tanstack/react-start";
import { findCustomerByEmail, getStripeClient } from "@/lib/stripe.server";

type CustomerPortalResponse = {
  url: string | null;
  error: string | null;
};

/**
 * Server function to get the Stripe Customer Portal URL.
 * Looks up the customer by email and creates a portal session.
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
    const customerId = await findCustomerByEmail(session.user.email);

    if (!customerId) {
      return {
        url: null,
        error: "Customer not found",
      };
    }

    const stripe = getStripeClient();
    const baseUrl = process.env.BETTER_AUTH_URL ?? "http://localhost:5173";

    const portalSession = await stripe.billingPortal.sessions.create({
      customer: customerId,
      return_url: `${baseUrl}/app/billing`,
    });

    return {
      url: portalSession.url,
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
