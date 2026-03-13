import { passkey } from "@better-auth/passkey";
import {
  checkout,
  polar,
  usage as polarUsage,
  portal,
  webhooks,
} from "@polar-sh/better-auth";
import { Polar } from "@polar-sh/sdk";
import { resend } from "@strait/mail/index.ts";
import { betterAuth } from "better-auth";
import { drizzleAdapter } from "better-auth/adapters/drizzle";
import { magicLink, oneTap, organization } from "better-auth/plugins";
import { tanstackStartCookies } from "better-auth/tanstack-start";
import { drizzle } from "drizzle-orm/postgres-js";
import postgres from "postgres";

const db = drizzle(postgres(process.env.AUTH_DATABASE_URL ?? ""));

const polarClient = new Polar({
  accessToken: process.env.POLAR_ACCESS_TOKEN ?? "",
  server:
    (process.env.POLAR_SERVER as "sandbox" | "production") ?? "production",
});

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
  database: drizzleAdapter(db, { provider: "pg" }),
  baseURL: process.env.BETTER_AUTH_URL,
  secret: process.env.BETTER_AUTH_SECRET,
  emailAndPassword: {
    enabled: true,
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
    }),
    magicLink({
      sendMagicLink: async ({ email, url }) => {
        await resend.emails.send({
          from: process.env.RESEND_SUPPORT_EMAIL ?? "noreply@strait.so",
          to: email,
          subject: "Sign in to Strait",
          html: `<p>Click the link below to sign in to Strait:</p><p><a href="${url}">Sign in to Strait</a></p><p>This link expires in 5 minutes. If you didn't request this, you can safely ignore this email.</p>`,
        });
      },
    }),
    passkey({
      rpID: new URL(process.env.BETTER_AUTH_URL ?? "http://localhost:3000")
        .hostname,
      rpName: "Strait",
      origin: process.env.BETTER_AUTH_URL ?? "http://localhost:3000",
    }),
    oneTap(),
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
              productId: process.env.POLAR_PROFESSIONAL_MONTHLY_ID ?? "",
              slug: "professional-monthly",
            },
            {
              productId: process.env.POLAR_PROFESSIONAL_YEARLY_ID ?? "",
              slug: "professional-yearly",
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
  ],
  user: {
    additionalFields: {
      defaultOrganizationId: {
        type: "string",
        required: false,
      },
    },
  },
});

export type Auth = typeof auth;
