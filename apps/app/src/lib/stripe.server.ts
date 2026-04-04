/**
 * Stripe server-side utilities for the Strait billing integration.
 *
 * Provides a lazily initialized Stripe SDK singleton and helpers for
 * customer lookup/creation. All functions are server-only and should
 * only be called from TanStack Start server functions or Better Auth hooks.
 *
 * @see https://docs.stripe.com/api — Stripe API reference
 * @see https://github.com/stripe/stripe-node — stripe-node SDK
 */
import Stripe from "stripe";

let _stripeClient: Stripe | null = null;

/**
 * Returns the lazily initialized Stripe SDK singleton.
 *
 * Deferred initialization is required because Cloudflare Workers only
 * populate `process.env` during request handling, not at module load time.
 *
 * @see https://developers.cloudflare.com/workers/runtime-apis/bindings/
 */
export const getStripeClient = (): Stripe => {
  if (!_stripeClient) {
    _stripeClient = new Stripe(process.env.STRIPE_SECRET_KEY ?? "", {
      apiVersion: "2026-03-25.dahlia",
    });
  }
  return _stripeClient;
};

/**
 * Look up an existing Stripe customer by email.
 *
 * @returns The Stripe customer ID, or `null` if no customer exists for the email.
 * @see https://docs.stripe.com/api/customers/list
 */
export const findCustomerByEmail = async (
  email: string
): Promise<string | null> => {
  const stripe = getStripeClient();
  const customers = await stripe.customers.list({ email, limit: 1 });
  return customers.data[0]?.id ?? null;
};

/**
 * Find an existing Stripe customer by email, or create one if none exists.
 * Prevents duplicate customers when multiple checkout sessions are created.
 *
 * @returns The Stripe customer ID (existing or newly created).
 * @see https://docs.stripe.com/api/customers/create
 */
export const findOrCreateCustomer = async (
  email: string,
  metadata?: Record<string, string>
): Promise<string> => {
  const existing = await findCustomerByEmail(email);
  if (existing) {
    return existing;
  }

  const stripe = getStripeClient();
  const customer = await stripe.customers.create({ email, metadata });
  return customer.id;
};
