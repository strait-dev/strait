import { oauthProvider } from "@better-auth/oauth-provider";
import { passkey } from "@better-auth/passkey";
import { type KeyLike, SignJWT, importPKCS8 } from "jose";
import {
  checkout,
  polar,
  usage as polarUsage,
  portal,
  webhooks,
} from "@polar-sh/better-auth";
import { Polar } from "@polar-sh/sdk";
import { render } from "@react-email/render";
import {
  ConfirmAccount,
  MagicLink,
  OrganizationInvite,
  ResetPassword,
} from "@strait/transactional";
import { betterAuth } from "better-auth";
import {
  jwt,
  magicLink,
  oneTap,
  organization,
  twoFactor,
} from "better-auth/plugins";
import { tanstackStartCookies } from "better-auth/tanstack-start";
import { Pool } from "pg";
import { ALL_OAUTH_SCOPES, STRAIT_API_SCOPES } from "@/lib/oauth-scopes";
import { resend } from "@/lib/resend.server";

export const authPool = new Pool({
  connectionString: process.env.AUTH_DATABASE_URL,
});

// Cache the OIDC private key import — importPKCS8 parses PEM and is CPU
// work that should happen once, not on every token sign. If import fails
// (e.g. invalid PEM), the cache is cleared so the next call retries
// instead of returning the rejected promise forever.
let oidcPrivateKeyPromise: Promise<KeyLike> | null = null;
function getOIDCPrivateKey(): Promise<KeyLike> {
  if (!oidcPrivateKeyPromise) {
    oidcPrivateKeyPromise = importPKCS8(
      process.env.OIDC_PRIVATE_KEY_PEM as string,
      "RS256"
    ).catch((err) => {
      oidcPrivateKeyPromise = null;
      throw err;
    });
  }
  return oidcPrivateKeyPromise;
}

const polarClient = process.env.POLAR_ACCESS_TOKEN
  ? new Polar({
      accessToken: process.env.POLAR_ACCESS_TOKEN,
      server:
        (process.env.POLAR_SERVER as "sandbox" | "production") ?? "production",
    })
  : null;

/**
 * Better Auth server instance.
 * Connects directly to the auth PostgreSQL database via Drizzle.
 * Handles: authentication, sessions, organizations, Polar billing.
 *
 * Supported auth methods:
 * - Email/password
 * - Magic link (via Resend)
 * - Passkey (WebAuthn)
 * - Google One Tap
 * - Google OAuth
 * - GitHub OAuth
 *
 * NOTE: We use betterAuth() directly instead of createServerAuth()
 * so TypeScript can infer the full plugin API types (organization methods, etc).
 */
export const auth = betterAuth({
  database: authPool,
  baseURL: process.env.BETTER_AUTH_URL,
  secret: process.env.BETTER_AUTH_SECRET,
  disabledPaths: ["/token"],
  emailAndPassword: {
    enabled: true,
    requireEmailVerification: true,
    sendResetPassword: async ({ user, url }) => {
      const html = await render(ResetPassword({ name: user.name, url }));
      await resend.emails.send({
        from: process.env.RESEND_SUPPORT_EMAIL ?? "noreply@strait.dev",
        to: user.email,
        subject: "Reset your Strait password",
        html,
      });
    },
  },
  emailVerification: {
    sendOnSignUp: true,
    sendVerificationEmail: async ({ user, url }) => {
      const html = await render(ConfirmAccount({ name: user.name, url }));
      await resend.emails.send({
        from: process.env.RESEND_SUPPORT_EMAIL ?? "noreply@strait.dev",
        to: user.email,
        subject: "Verify your email for Strait",
        html,
      });
    },
  },
  account: {
    accountLinking: {
      enabled: true,
      trustedProviders: ["google", "github"],
    },
  },
  socialProviders: {
    google: {
      clientId: process.env.GOOGLE_CLIENT_ID ?? "",
      clientSecret: process.env.GOOGLE_CLIENT_SECRET ?? "",
    },
    github: {
      clientId: process.env.GITHUB_CLIENT_ID ?? "",
      clientSecret: process.env.GITHUB_CLIENT_SECRET ?? "",
    },
  },
  plugins: [
    tanstackStartCookies(),
    organization({
      allowUserToCreateOrganization: true,
      sendInvitationEmail: async (data) => {
        const inviteLink = `${process.env.BETTER_AUTH_URL}/invitation/${data.id}`;
        const html = await render(
          OrganizationInvite({
            name: data.inviter.user.name,
            orgName: data.organization.name,
            inviteLink,
          })
        );
        await resend.emails.send({
          from: process.env.RESEND_SUPPORT_EMAIL ?? "noreply@strait.dev",
          to: data.email,
          subject: `You've been invited to ${data.organization.name}`,
          html,
        });
      },
    }),
    magicLink({
      sendMagicLink: async ({ email, url }) => {
        const html = await render(MagicLink({ email, url }));
        await resend.emails.send({
          from: process.env.RESEND_SUPPORT_EMAIL ?? "noreply@strait.dev",
          to: email,
          subject: "Sign in to Strait",
          html,
        });
      },
    }),
    passkey({
      rpID: new URL(process.env.BETTER_AUTH_URL ?? "http://localhost:5173")
        .hostname,
      rpName: "Strait",
      origin: process.env.BETTER_AUTH_URL ?? "http://localhost:5173",
    }),
    oneTap(),
    twoFactor(),
    jwt({
      jwks: {
        keyPairConfig: { alg: "RS256" },
        // Point to our own JWKS endpoint so the Go service can fetch
        // the public key dynamically if needed.
        remoteUrl: `${process.env.BETTER_AUTH_URL ?? "http://localhost:5173"}/api/auth/jwks`,
      },
      jwt: {
        issuer: process.env.OIDC_ISSUER,
        audience: process.env.OIDC_AUDIENCE,
        // Custom sign function: signs with the RSA private key from env
        // instead of the auto-generated key pair in the database. This
        // ensures the Go OIDC verifier (which holds the matching public
        // key) can validate all tokens.
        sign: process.env.OIDC_PRIVATE_KEY_PEM
          ? async (payload) => {
              const privateKey = await getOIDCPrivateKey();
              return new SignJWT(payload)
                .setProtectedHeader({
                  alg: "RS256",
                  typ: "JWT",
                  kid: "oidc-rsa-1",
                })
                .sign(privateKey);
            }
          : undefined,
      },
    }),
    oauthProvider({
      loginPage: "/login",
      consentPage: "/oauth/consent",
      // List of valid audiences for JWT access tokens. Without this, the
      // plugin issues opaque tokens instead of JWTs. The Go OIDC verifier
      // expects JWTs signed with RS256.
      validAudiences: process.env.OIDC_AUDIENCE
        ? [process.env.OIDC_AUDIENCE]
        : undefined,
      scopes: [...ALL_OAUTH_SCOPES],
      allowDynamicClientRegistration: true,
      allowUnauthenticatedClientRegistration: true,
      clientRegistrationDefaultScopes: [
        "openid",
        "profile",
        "jobs:read",
        "runs:read",
        "stats:read",
      ],
      clientRegistrationAllowedScopes: [...STRAIT_API_SCOPES],
      accessTokenExpiresIn: 900, // 15 minutes — short-lived for security
      refreshTokenExpiresIn: 2_592_000, // 30 days
      codeExpiresIn: 600, // 10 minutes
      rateLimit: {
        token: { window: 60, max: 20 },
        authorize: { window: 60, max: 30 },
        register: { window: 60, max: 5 },
        revoke: { window: 60, max: 30 },
        userinfo: { window: 60, max: 60 },
        introspect: { window: 60, max: 100 },
      },
    }),
    // SSO disabled: @better-auth/sso has a known ESM incompatibility
    // (samlify requires camelcase@9 ESM-only from CJS). Re-enable when
    // https://github.com/better-auth/better-auth/issues/8620 is fixed.
    ...(polarClient
      ? [
          polar({
            client: polarClient,
            createCustomerOnSignUp: true,
            use: [
              checkout({
                products: [
                  {
                    productId: process.env.POLAR_STARTER_MONTHLY_ID ?? "",
                    slug: "starter-monthly",
                  },
                  {
                    productId: process.env.POLAR_STARTER_YEARLY_ID ?? "",
                    slug: "starter-yearly",
                  },
                  {
                    productId: process.env.POLAR_PRO_MONTHLY_ID ?? "",
                    slug: "pro-monthly",
                  },
                  {
                    productId: process.env.POLAR_PRO_YEARLY_ID ?? "",
                    slug: "pro-yearly",
                  },
                ],
                successUrl: "/app?checkout_success=true",
                authenticatedUsersOnly: true,
              }),
              portal({
                returnUrl: process.env.BETTER_AUTH_URL,
              }),
              polarUsage(),
              ...(process.env.POLAR_APP_WEBHOOK_SECRET
                ? [
                    webhooks({
                      secret: process.env.POLAR_APP_WEBHOOK_SECRET,
                    }),
                  ]
                : []),
            ] as any,
          }),
        ]
      : []),
  ],
  databaseHooks: {
    user: {
      create: {
        after: async (user) => {
          // Auto-create a workspace (organization) for every new user.
          // This ensures users always have an org for billing enforcement
          // and project creation, without requiring an onboarding step.
          try {
            const workspaceName = user.name
              ? `${user.name}'s Workspace`
              : "My Workspace";
            const slug = `ws-${crypto.randomUUID().replace(/-/g, "").slice(0, 12)}`;

            // Server-side call: pass userId directly (no session headers needed).
            const org = await auth.api.createOrganization({
              body: { name: workspaceName, slug, userId: user.id },
            });

            if (org) {
              await authPool.query(
                `UPDATE "user" SET "defaultOrganizationId" = $1 WHERE id = $2`,
                [org.id, user.id]
              );

              // Auto-create a default project so the user lands on a
              // ready-to-use dashboard instead of an empty "Create project" screen.
              try {
                await authPool.query(`
                  CREATE TABLE IF NOT EXISTS project (
                    id TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
                    organization_id TEXT NOT NULL,
                    name TEXT NOT NULL,
                    slug TEXT NOT NULL,
                    description TEXT DEFAULT '',
                    created_by TEXT NOT NULL,
                    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
                    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
                    UNIQUE(organization_id, slug)
                  )
                `);

                const projectId = crypto.randomUUID();
                const projectSlug = `project-${projectId.slice(0, 8)}`;

                await authPool.query(
                  `INSERT INTO project (id, organization_id, name, slug, created_by)
                   VALUES ($1, $2, $3, $4, $5)
                   ON CONFLICT DO NOTHING`,
                  [projectId, org.id, "Default Project", projectSlug, user.id]
                );

                // Set as the user's active project.
                await authPool.query(
                  `UPDATE "user" SET "activeProjectId" = $1 WHERE id = $2`,
                  [projectId, user.id]
                );

                // Sync to Go API (best-effort, don't fail signup).
                const apiUrl =
                  process.env.STRAIT_API_URL || "http://localhost:8080";
                const secret = process.env.INTERNAL_SECRET;
                if (secret) {
                  fetch(`${apiUrl}/v1/projects`, {
                    method: "POST",
                    headers: {
                      "Content-Type": "application/json",
                      "X-Internal-Secret": secret,
                    },
                    body: JSON.stringify({
                      id: projectId,
                      org_id: org.id,
                      name: "Default Project",
                    }),
                  }).catch(() => {
                    // Best-effort sync; don't fail signup if Go API is down.
                  });
                }
              } catch (projectErr) {
                console.error(
                  "Failed to auto-create default project for user",
                  user.id,
                  projectErr
                );
              }
            }
          } catch (err) {
            console.error(
              "Failed to auto-create workspace for user",
              user.id,
              err
            );
            // TODO: Add Sentry capture and reconciliation job for users
            // that end up without an organization due to this failure.
          }
        },
      },
    },
    session: {
      create: {
        before: async (session) => {
          const result = await authPool.query<{
            defaultOrganizationId: string | null;
          }>(`SELECT "defaultOrganizationId" FROM "user" WHERE id = $1`, [
            session.userId,
          ]);
          const defaultOrgId = result.rows[0]?.defaultOrganizationId;
          if (typeof defaultOrgId === "string" && defaultOrgId) {
            return {
              data: {
                ...session,
                activeOrganizationId: defaultOrgId,
              },
            };
          }
          return { data: session };
        },
      },
    },
  },
  user: {
    additionalFields: {
      defaultOrganizationId: {
        type: "string",
        required: false,
      },
      activeProjectId: {
        type: "string",
        required: false,
      },
    },
  },
});

export type Auth = typeof auth;
