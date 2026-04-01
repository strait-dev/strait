import { env as cfEnv } from "cloudflare:workers";
import { oauthProvider } from "@better-auth/oauth-provider";
import { passkey } from "@better-auth/passkey";
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
import { importPKCS8, SignJWT } from "jose";
import { Client, type Pool } from "pg";
import {
  ALL_OAUTH_SCOPES,
  DEFAULT_REGISTRATION_SCOPES,
  OAUTH_CONSENT_PAGE,
  OAUTH_LOGIN_PAGE,
  OIDC_ALGORITHM,
  OIDC_KEY_ID,
  STRAIT_API_SCOPES,
} from "@/lib/oauth-scopes";
import { getResend } from "@/lib/resend.server";

// ---------------------------------------------------------------------------
// Deferred singletons
//
// Cloudflare Workers only populate `process.env` during request handling,
// not at module load time. Every constructor that reads an env var must be
// wrapped in a lazy getter so it runs on the first request, not on import.
// ---------------------------------------------------------------------------

/**
 * Resolve the auth database connection string.
 *
 * In Cloudflare Workers, uses the Hyperdrive binding which provides a
 * proxied connection string. Without Hyperdrive, `pg` cannot establish
 * raw TCP connections from the Workers runtime and will hang.
 *
 * Falls back to `AUTH_DATABASE_URL` for local development where
 * Hyperdrive provides a local connection string automatically.
 */
function getAuthConnectionString(): string {
  const hyperdrive = (cfEnv as Record<string, unknown>).HYPERDRIVE as
    | { connectionString: string }
    | undefined;
  console.log("[auth] Hyperdrive binding present:", !!hyperdrive);
  console.log(
    "[auth] Hyperdrive connectionString present:",
    !!hyperdrive?.connectionString
  );
  if (hyperdrive?.connectionString) {
    console.log("[auth] Using Hyperdrive connection string");
    return hyperdrive.connectionString;
  }
  const fallback = process.env.AUTH_DATABASE_URL ?? "";
  console.log("[auth] Falling back to AUTH_DATABASE_URL, present:", !!fallback);
  return fallback;
}

/**
 * Lazily initialized PostgreSQL connection pool for the auth database.
 *
 * Uses Hyperdrive in production for proxied TCP connections, falling
 * back to `AUTH_DATABASE_URL` for local development.
 */
/**
 * Returns a pg-compatible object that creates a fresh Client per query.
 *
 * Cloudflare Hyperdrive docs recommend `new Client()` per request —
 * Pool maintains persistent connections that conflict with Hyperdrive's
 * per-request proxy model and cause queries to hang indefinitely.
 *
 * The returned object implements the `query()` and `connect()` methods
 * that Better Auth's Kysely adapter requires.
 */
export function getAuthPool(): Pool {
  const connectionString = getAuthConnectionString();
  return {
    async query(text: string, values?: unknown[]) {
      const client = new Client({ connectionString });
      try {
        await client.connect();
        return await client.query(text, values);
      } finally {
        client.end().catch(() => {
          // Swallow disconnect errors — the query already completed
        });
      }
    },
    async connect() {
      const client = new Client({ connectionString });
      await client.connect();
      return {
        query: client.query.bind(client),
        release: () => {
          client.end().catch(() => {
            // Swallow disconnect errors
          });
        },
      };
    },
  } as unknown as Pool;
}

/**
 * Cache the OIDC private key import. `importPKCS8` parses PEM and is CPU
 * work that should happen once, not on every token sign. If import fails
 * (e.g. invalid PEM), the cache is cleared so the next call retries
 * instead of returning the rejected promise forever.
 */
let oidcPrivateKeyPromise: Promise<CryptoKey> | null = null;

function getOIDCPrivateKey(): Promise<CryptoKey> {
  if (!oidcPrivateKeyPromise) {
    oidcPrivateKeyPromise = importPKCS8(
      process.env.OIDC_PRIVATE_KEY_PEM as string,
      OIDC_ALGORITHM
    ).catch((err) => {
      oidcPrivateKeyPromise = null;
      throw err;
    });
  }
  return oidcPrivateKeyPromise;
}

/**
 * Create a Stripe customer for a newly signed-up user.
 * Links the org_id in metadata so the Go webhook handler can resolve orgs.
 * Best-effort: errors are logged but never fail signup.
 */
async function createStripeCustomer(
  user: { id: string; email: string; name: string },
  orgId: string
): Promise<void> {
  if (!process.env.STRIPE_SECRET_KEY) return;
  try {
    const { default: Stripe } = await import("stripe");
    const stripe = new Stripe(process.env.STRIPE_SECRET_KEY);
    await stripe.customers.create({
      email: user.email,
      name: user.name || undefined,
      metadata: { org_id: orgId, user_id: user.id },
    });
  } catch (err) {
    console.error("Failed to create Stripe customer for user", user.id, err);
  }
}

/**
 * Build the Better Auth configuration.
 *
 * This is called lazily on the first request via {@link getAuth} because
 * the entire config tree reads from `process.env`, which is empty at
 * module load time in Cloudflare Workers.
 *
 * Handles: authentication, sessions, organizations.
 *
 * Supported auth methods:
 * - Email/password
 * - Magic link (via Resend)
 * - Passkey (WebAuthn)
 * - Google One Tap
 * - Google OAuth
 * - GitHub OAuth
 */
function createAuth() {
  const pool = getAuthPool();
  const resend = getResend();
  return betterAuth({
    database: pool,
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
      // Store OAuth state in an encrypted cookie instead of the database.
      // This avoids database queries during the OAuth callback, which
      // is more reliable on Cloudflare Workers where the database
      // connection may not persist between the sign-in and callback requests.
      storeStateStrategy: "cookie",
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
          keyPairConfig: { alg: OIDC_ALGORITHM },
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
                    alg: OIDC_ALGORITHM,
                    typ: "JWT",
                    kid: OIDC_KEY_ID,
                  })
                  .sign(privateKey);
              }
            : undefined,
        },
      }),
      oauthProvider({
        loginPage: OAUTH_LOGIN_PAGE,
        consentPage: OAUTH_CONSENT_PAGE,
        // List of valid audiences for JWT access tokens. Without this, the
        // plugin issues opaque tokens instead of JWTs. The Go OIDC verifier
        // expects JWTs signed with RS256.
        validAudiences: process.env.OIDC_AUDIENCE
          ? [process.env.OIDC_AUDIENCE]
          : undefined,
        scopes: [...ALL_OAUTH_SCOPES],
        allowDynamicClientRegistration: true,
        allowUnauthenticatedClientRegistration: true,
        clientRegistrationDefaultScopes: [...DEFAULT_REGISTRATION_SCOPES],
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
      // Stripe billing is handled via standalone server functions (checkout,
      // portal) and a Go backend webhook handler, not through Better Auth plugins.
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
              const org = await (await getAuth()).api.createOrganization({
                body: { name: workspaceName, slug, userId: user.id },
              });

              if (org) {
                await pool.query(
                  `UPDATE "user" SET "defaultOrganizationId" = $1 WHERE id = $2`,
                  [org.id, user.id]
                );

                // Create a Stripe customer (best-effort, don't fail signup).
                await createStripeCustomer(user, org.id);

                // Auto-create a default project so the user lands on a
                // ready-to-use dashboard instead of an empty "Create project" screen.
                try {
                  await pool.query(`
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

                  await pool.query(
                    `INSERT INTO project (id, organization_id, name, slug, created_by)
                   VALUES ($1, $2, $3, $4, $5)
                   ON CONFLICT DO NOTHING`,
                    [projectId, org.id, "Default Project", projectSlug, user.id]
                  );

                  // Set as the user's active project.
                  await pool.query(
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
            const result = await pool.query<{
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
}

// ---------------------------------------------------------------------------
// Exported lazy singletons
// ---------------------------------------------------------------------------

/**
 * Lazily initialized Better Auth server instance.
 *
 * The full `betterAuth()` config reads dozens of `process.env` values.
 * In Cloudflare Workers these are only available during request handling,
 * so we defer the entire construction to the first call.
 */
type AuthInstance = ReturnType<typeof createAuth>;

let _auth: AuthInstance | null = null;

/**
 * Returns the Better Auth singleton, initializing it on first call.
 * Callers must `await` the result for backwards compatibility.
 */
export function getAuth(): Promise<AuthInstance> {
  if (!_auth) {
    _auth = createAuth();
  }
  return Promise.resolve(_auth);
}
