import { queryOptions, useMutation, useQueryClient } from "@tanstack/react-query";
import { createServerFn } from "@tanstack/react-start";
import { z } from "zod";
import { queryKeys } from "@/hooks/query-keys";
import { DEFAULT_GC_TIME, DEFAULT_STALE_TIME } from "@/hooks/utils";
import { auth } from "@/lib/auth.server";
import { authMiddleware } from "@/middlewares/auth";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export type OAuthConsentItem = {
  id: string;
  clientId: string;
  clientName: string;
  scopes: string;
  createdAt: string;
  updatedAt: string;
};

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function formatScopes(scopes: unknown): string {
  if (typeof scopes === "string") {
    return scopes;
  }
  if (Array.isArray(scopes)) {
    return scopes.join(",");
  }
  return "";
}

// ---------------------------------------------------------------------------
// Server functions
// ---------------------------------------------------------------------------

export const fetchOAuthConsents = createServerFn({ method: "GET" })
  .middleware([authMiddleware])
  .handler(async () => {
    const consents = (await auth.api.getOAuthConsents()) ?? [];
    const items: OAuthConsentItem[] = [];
    for (const consent of consents as any[]) {
      let clientName = "Unknown Application";
      try {
        const client = await (auth.api as any).getOAuthClient({
          body: { client_id: consent.clientId },
        });
        if (client?.name) {
          clientName = client.name;
        }
      } catch {
        // Client may have been deleted — use fallback name.
      }
      items.push({
        id: consent.id,
        clientId: consent.clientId,
        clientName,
        scopes: formatScopes(consent.scopes),
        createdAt:
          consent.createdAt?.toISOString?.() ?? String(consent.createdAt),
        updatedAt:
          consent.updatedAt?.toISOString?.() ?? String(consent.updatedAt),
      });
    }
    return items;
  });

export const revokeOAuthConsentFn = createServerFn({ method: "POST" })
  .inputValidator(z.object({ consentId: z.string().min(1) }))
  .middleware([authMiddleware])
  .handler(async ({ data }) => {
    await auth.api.deleteOAuthConsent({
      body: { id: data.consentId },
    });
    return { success: true };
  });

// ---------------------------------------------------------------------------
// Query options
// ---------------------------------------------------------------------------

export const oauthConsentsQueryOptions = () =>
  queryOptions({
    queryKey: queryKeys.oauthConsents.list.queryKey,
    queryFn: () => fetchOAuthConsents(),
    staleTime: DEFAULT_STALE_TIME,
    gcTime: DEFAULT_GC_TIME,
  });

// ---------------------------------------------------------------------------
// Mutations
// ---------------------------------------------------------------------------

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
