import { createFileRoute } from "@tanstack/react-router";
import { exportJWK, importSPKI, importPKCS8 } from "jose";
import { captureException } from "@/lib/sentry";

const CORS_HEADERS = {
  "Access-Control-Allow-Origin": "*",
  "Access-Control-Allow-Methods": "GET, OPTIONS",
  "Cache-Control": "public, max-age=3600",
};

/**
 * JWKS endpoint serving the RSA public key for OIDC token verification.
 *
 * Derives the public key from OIDC_PRIVATE_KEY_PEM (or uses OIDC_PUBLIC_KEY_PEM
 * directly if available). This allows the Go OIDC verifier and any other
 * resource server to fetch the public key dynamically.
 */
export const Route = createFileRoute("/api/auth/jwks")({
  server: {
    handlers: {
      GET: async () => {
        try {
          let publicJwk: Record<string, unknown> | undefined;

          if (process.env.OIDC_PRIVATE_KEY_PEM) {
            const privateKey = await importPKCS8(
              process.env.OIDC_PRIVATE_KEY_PEM,
              "RS256"
            );
            // Extract public key components from the private key
            const jwk = await exportJWK(privateKey);
            // Strip private key fields — only expose public components
            publicJwk = {
              kty: jwk.kty,
              n: jwk.n,
              e: jwk.e,
              alg: "RS256",
              use: "sig",
              kid: "oidc-rsa-1",
            };
          } else if (process.env.OIDC_PUBLIC_KEY_PEM) {
            const publicKey = await importSPKI(
              process.env.OIDC_PUBLIC_KEY_PEM,
              "RS256"
            );
            const jwk = await exportJWK(publicKey);
            publicJwk = {
              ...jwk,
              alg: "RS256",
              use: "sig",
              kid: "oidc-rsa-1",
            };
          } else {
            return new Response(JSON.stringify({ keys: [] }), {
              headers: { "Content-Type": "application/json", ...CORS_HEADERS },
            });
          }

          return new Response(JSON.stringify({ keys: [publicJwk] }), {
            headers: { "Content-Type": "application/json", ...CORS_HEADERS },
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
        new Response(null, { status: 204, headers: CORS_HEADERS }),
    },
  },
});
