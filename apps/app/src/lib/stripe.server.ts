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
import { Data, Effect } from "effect";
import Stripe from "stripe";

const STRIPE_API_VERSION = "2026-06-24.dahlia";
const CUSTOMER_LIST_LIMIT = 100;
const ORG_ID_METADATA_KEY = "org_id";
const USER_ID_METADATA_KEY = "user_id";

let _stripeClient: Stripe | null = null;

export type StripeIntegrationErrorOperation =
  | "customer_lookup"
  | "customer_create";

/** Typed failure for server-side Stripe integration calls. */
export class StripeIntegrationError extends Data.TaggedError(
  "StripeIntegrationError"
)<{
  readonly operation: StripeIntegrationErrorOperation;
  readonly message: string;
  readonly orgId: string;
  readonly email: string;
  readonly cause?: unknown;
}> {}

type FindOrCreateCustomerForOrgInput = {
  email: string;
  orgId: string;
  userId?: string;
  name?: string | null;
};

/** Converts typed Stripe failures into the legacy `Error` shape used by Promise callers. */
export function stripeIntegrationErrorToError(
  error: StripeIntegrationError
): Error {
  return new Error(error.message, { cause: error });
}

/**
 * Returns the lazily initialized Stripe SDK singleton.
 *
 * Deferred initialization keeps tests and production startup from reading env
 * before the process has injected deployment variables.
 */
export const getStripeClient = (): Stripe => {
  if (!_stripeClient) {
    _stripeClient = new Stripe(process.env.STRIPE_SECRET_KEY ?? "", {
      apiVersion: STRIPE_API_VERSION,
    });
  }
  return _stripeClient;
};

function runStripeEffect<T>(
  effect: Effect.Effect<T, StripeIntegrationError>
): Promise<T> {
  return Effect.runPromise(
    effect.pipe(
      Effect.catchAll((error) =>
        Effect.die(stripeIntegrationErrorToError(error))
      )
    )
  );
}

/**
 * Finds the Stripe customer whose email and organization metadata match the active org.
 */
export const findCustomerByOrgEffect = (
  email: string,
  orgId: string
): Effect.Effect<string | null, StripeIntegrationError> =>
  Effect.tryPromise({
    try: async () => {
      const stripe = getStripeClient();
      const customers = await stripe.customers.list({
        email,
        limit: CUSTOMER_LIST_LIMIT,
      });
      const match = customers.data.find(
        (customer) => customer.metadata?.[ORG_ID_METADATA_KEY] === orgId
      );
      return match?.id ?? null;
    },
    catch: (cause) =>
      new StripeIntegrationError({
        operation: "customer_lookup",
        message: "Stripe customer lookup failed",
        orgId,
        email,
        cause,
      }),
  });

/** Promise adapter for {@link findCustomerByOrgEffect}. */
export const findCustomerByOrg = (
  email: string,
  orgId: string
): Promise<string | null> =>
  runStripeEffect(findCustomerByOrgEffect(email, orgId));

/**
 * Finds the existing org customer or creates one with Strait metadata.
 */
export const findOrCreateCustomerForOrgEffect = ({
  email,
  orgId,
  userId,
  name,
}: FindOrCreateCustomerForOrgInput): Effect.Effect<
  string,
  StripeIntegrationError
> =>
  Effect.gen(function* () {
    const existing = yield* findCustomerByOrgEffect(email, orgId);
    if (existing) {
      return existing;
    }

    const stripe = getStripeClient();
    const customer = yield* Effect.tryPromise({
      try: () =>
        stripe.customers.create({
          email,
          name: name ?? undefined,
          metadata: {
            [ORG_ID_METADATA_KEY]: orgId,
            ...(userId ? { [USER_ID_METADATA_KEY]: userId } : {}),
          },
        }),
      catch: (cause) =>
        new StripeIntegrationError({
          operation: "customer_create",
          message: "Stripe customer creation failed",
          orgId,
          email,
          cause,
        }),
    });
    return customer.id;
  });

/** Promise adapter for {@link findOrCreateCustomerForOrgEffect}. */
export const findOrCreateCustomerForOrg = ({
  email,
  orgId,
  userId,
  name,
}: FindOrCreateCustomerForOrgInput): Promise<string> =>
  runStripeEffect(
    findOrCreateCustomerForOrgEffect({ email, orgId, userId, name })
  );
