import { createServerFn } from "@tanstack/react-start";
import { getRequestHeaders } from "@tanstack/react-start/server";
import { getAuth } from "@/lib/auth.server";
import { isCommunityEdition } from "@/lib/edition";
import { findCustomerByOrg, getStripeClient } from "@/lib/stripe.server";
import { requireOrgAdmin } from "@/middlewares/require-access";

type CustomerPortalResponse = {
  url: string | null;
  error: string | null;
};

/**
 * Server function to get the Stripe Customer portal URL.
 * Looks up the customer by email and creates a portal session.
 *
 * Community edition returns an error payload — the customer portal
 * is a cloud-only feature and self-host users have no Stripe
 * customers to manage.
 */
export const getCustomerPortalUrlServerFn = createServerFn({
  method: "GET",
}).handler(async (): Promise<CustomerPortalResponse> => {
  if (isCommunityEdition) {
    return {
      url: null,
      error: "Customer portal is not available in community edition",
    };
  }

  const headers = getRequestHeaders();
  const session = await (await getAuth()).api.getSession({ headers });
  const email = session?.user?.email;
  const orgId = (session?.session as Record<string, unknown> | undefined)
    ?.activeOrganizationId;

  if (!(session?.user?.id && email && typeof orgId === "string" && orgId)) {
    return {
      url: null,
      error: "Session not found",
    };
  }

  try {
    await requireOrgAdmin(session.user.id, orgId);
    const customerId = await findCustomerByOrg(email, orgId);

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
