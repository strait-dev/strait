import Stripe from "stripe";

/**
 * Lazily initialized Stripe client singleton.
 * Deferred because Cloudflare Workers only populate `process.env` during
 * request handling, not at module load time.
 */
let _stripeClient: Stripe | null = null;

export function getStripeClient(): Stripe {
  if (!_stripeClient) {
    _stripeClient = new Stripe(process.env.STRIPE_SECRET_KEY ?? "", {
      apiVersion: "2025-08-27.basil",
    });
  }
  return _stripeClient;
}

/**
 * Look up an existing Stripe customer by email.
 * Returns the customer ID or null if not found.
 */
export async function findCustomerByEmail(
  email: string
): Promise<string | null> {
  const stripe = getStripeClient();
  const customers = await stripe.customers.list({ email, limit: 1 });
  return customers.data[0]?.id ?? null;
}

/**
 * Find or create a Stripe customer by email.
 * Avoids duplicate customers by checking first.
 */
export async function findOrCreateCustomer(
  email: string,
  metadata?: Record<string, string>
): Promise<string> {
  const existing = await findCustomerByEmail(email);
  if (existing) return existing;

  const stripe = getStripeClient();
  const customer = await stripe.customers.create({ email, metadata });
  return customer.id;
}
