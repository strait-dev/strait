import { createFileRoute } from "@tanstack/react-router";
import { exportJWK, importPKCS8, importSPKI } from "jose";
import {
  OAUTH_CORS_HEADERS,
  OIDC_ALGORITHM,
  OIDC_KEY_ID,
} from "@/lib/oauth-scopes";
import { captureException } from "@/lib/sentry";

export const Route = createFileRoute("/api/auth/jwks")({
  server: {
    handlers: {
      GET: async () => {
        try {
          let publicJwk: Record<string, unknown> | undefined;

          if (process.env.OIDC_PRIVATE_KEY_PEM) {
            const privateKey = await importPKCS8(
              process.env.OIDC_PRIVATE_KEY_PEM,
              OIDC_ALGORITHM
            );
            const jwk = await exportJWK(privateKey);
            publicJwk = {
              kty: jwk.kty,
              n: jwk.n,
              e: jwk.e,
              alg: OIDC_ALGORITHM,
              use: "sig",
              kid: OIDC_KEY_ID,
            };
          } else if (process.env.OIDC_PUBLIC_KEY_PEM) {
            const publicKey = await importSPKI(
              process.env.OIDC_PUBLIC_KEY_PEM,
              OIDC_ALGORITHM
            );
            const jwk = await exportJWK(publicKey);
            publicJwk = {
              ...jwk,
              alg: OIDC_ALGORITHM,
              use: "sig",
              kid: OIDC_KEY_ID,
            };
          } else {
            return new Response(JSON.stringify({ keys: [] }), {
              headers: {
                "Content-Type": "application/json",
                ...OAUTH_CORS_HEADERS,
              },
            });
          }

          return new Response(JSON.stringify({ keys: [publicJwk] }), {
            headers: {
              "Content-Type": "application/json",
              ...OAUTH_CORS_HEADERS,
            },
          });
        } catch (err) {
          captureException(err, { tags: { feature: "oauth", action: "jwks" } });
          return new Response(JSON.stringify({ keys: [] }), {
            status: 500,
            headers: { "Content-Type": "application/json" },
          });
        }
      },
      OPTIONS: () =>
        new Response(null, { status: 204, headers: OAUTH_CORS_HEADERS }),
    },
  },
});
