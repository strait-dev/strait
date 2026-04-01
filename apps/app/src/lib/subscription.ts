import { createServerFn } from "@tanstack/react-start";
import Stripe from "stripe";

/**
 * Lazily initialized Stripe client singleton.
 * Deferred because Cloudflare Workers only populate `process.env` during
 * request handling, not at module load time.
 */
let _stripeClient: Stripe | null = null;

function getStripeClient(): Stripe {
  if (!_stripeClient) {
    _stripeClient = new Stripe(process.env.STRIPE_SECRET_KEY ?? "", {
      apiVersion: "2025-08-27.basil",
    });
  }
  return _stripeClient;
}

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
    const stripe = getStripeClient();

    // Look up the Stripe customer by email
    const customers = await stripe.customers.list({
      email: session.user.email,
      limit: 1,
    });

    if (customers.data.length === 0) {
      return {
        url: null,
        error: "Customer not found",
      };
    }

    const customerId = customers.data[0].id;

    // Create a Stripe Billing Portal session
    const portalSession = await stripe.billingPortal.sessions.create({
      customer: customerId,
      return_url: process.env.BETTER_AUTH_URL ?? "http://localhost:5173",
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
