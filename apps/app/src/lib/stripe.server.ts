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
import "reflect-metadata";
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
      apiVersion: "2026-04-22.dahlia",
    });
  }
  return _stripeClient;
};

export const findCustomerByOrg = async (
  email: string,
  orgId: string
): Promise<string | null> => {
  const stripe = getStripeClient();
  const customers = await stripe.customers.list({ email, limit: 100 });
  const match = customers.data.find(
    (customer) => customer.metadata?.org_id === orgId
  );
  return match?.id ?? null;
};

export const findOrCreateCustomerForOrg = async ({
  email,
  orgId,
  userId,
  name,
}: {
  email: string;
  orgId: string;
  userId?: string;
  name?: string | null;
}): Promise<string> => {
  const existing = await findCustomerByOrg(email, orgId);
  if (existing) {
    return existing;
  }

  const stripe = getStripeClient();
  const customer = await stripe.customers.create({
    email,
    name: name ?? undefined,
    metadata: {
      org_id: orgId,
      ...(userId ? { user_id: userId } : {}),
    },
  });
  return customer.id;
};
