import {
  queryOptions,
  useMutation,
  useQueryClient,
} from "@tanstack/react-query";
import { createServerFn } from "@tanstack/react-start";
import { getRequestHeaders } from "@tanstack/react-start/server";
import { z } from "zod";
import { queryKeys } from "@/hooks/query-keys";
import { DEFAULT_GC_TIME, DEFAULT_STALE_TIME } from "@/hooks/utils";
import { getAuth } from "@/lib/auth.server";
import { parseAndFilterScopes } from "@/lib/oauth-scopes";
import { captureException } from "@/lib/sentry";
import { authMiddleware } from "@/middlewares/auth";

export type OAuthConsentItem = {
  id: string;
  clientId: string;
  clientName: string;
  scopes: string;
  createdAt: string;
  updatedAt: string;
};

const fetchOAuthConsents = createServerFn({ method: "GET" })
  .middleware([authMiddleware])
  .handler(async () => {
    const headers = getRequestHeaders();
    const consents =
      (await (await getAuth()).api.getOAuthConsents({ headers })) ?? [];
    const items: OAuthConsentItem[] = [];
    for (const consent of consents as any[]) {
      let clientName = "Unknown application";
      try {
        const client = await (
          (await getAuth()).api as any
        ).getOAuthClientPublic({
          query: { client_id: consent.clientId },
          headers,
        });
        if (client?.client_name) {
          clientName = client.client_name;
        }
      } catch (err) {
        captureException(err, {
          tags: { feature: "oauth", action: "fetch_client" },
        });
      }
      items.push({
        id: consent.id,
        clientId: consent.clientId,
        clientName,
        scopes: parseAndFilterScopes(consent.scopes).join(","),
        createdAt:
          consent.createdAt?.toISOString?.() ?? String(consent.createdAt),
        updatedAt:
          consent.updatedAt?.toISOString?.() ?? String(consent.updatedAt),
      });
    }
    return items;
  });

const revokeOAuthConsentFn = createServerFn({ method: "POST" })
  .inputValidator(z.object({ consentId: z.string().min(1) }))
  .middleware([authMiddleware])
  .handler(async ({ data }) => {
    await (await getAuth()).api.deleteOAuthConsent({
      body: { id: data.consentId },
      headers: getRequestHeaders(),
    });
    return { success: true };
  });

export const oauthConsentsQueryOptions = () =>
  queryOptions({
    queryKey: queryKeys.oauthConsents.list.queryKey,
    queryFn: () => fetchOAuthConsents(),
    staleTime: DEFAULT_STALE_TIME,
    gcTime: DEFAULT_GC_TIME,
  });

export const useRevokeOAuthConsent = () => {
  const queryClient = useQueryClient();
  return useMutation({
    mutationKey: ["oauthConsents", "revoke"],
    mutationFn: (consentId: string) =>
      revokeOAuthConsentFn({ data: { consentId } }),
    onSettled: () => {
      queryClient.invalidateQueries({
        queryKey: queryKeys.oauthConsents._def,
      });
    },
  });
};
