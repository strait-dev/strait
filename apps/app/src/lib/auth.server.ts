import { passkey } from "@better-auth/passkey";
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
  magicLink,
  oneTap,
  organization,
  twoFactor,
} from "better-auth/plugins";
import { tanstackStartCookies } from "better-auth/tanstack-start";
import { Pool } from "pg";
import { resend } from "@/lib/resend.server";

export const authPool = new Pool({
  connectionString: process.env.AUTH_DATABASE_URL,
});

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
              ...(process.env.POLAR_WEBHOOK_SECRET
                ? [
                    webhooks({
                      secret: process.env.POLAR_WEBHOOK_SECRET,
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
            const slug = `ws-${user.id.slice(0, 8)}`;

            const org = await auth.api.createOrganization({
              body: { name: workspaceName, slug },
              headers: new Headers(),
            });

            if (org) {
              await authPool.query(
                `UPDATE "user" SET "defaultOrganizationId" = $1, "onboarded" = true WHERE id = $2`,
                [org.id, user.id]
              );
            }
          } catch (err) {
            console.error("Failed to auto-create workspace for user", user.id, err);
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
      onboarded: {
        type: "boolean",
        required: false,
        defaultValue: false,
      },
    },
  },
});

export type Auth = typeof auth;
