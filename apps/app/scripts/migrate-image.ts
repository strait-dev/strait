import { oauthProvider } from "@better-auth/oauth-provider";
import { passkey } from "@better-auth/passkey";
import { betterAuth } from "better-auth";
import { getMigrations } from "better-auth/db/migration";
import {
  jwt,
  magicLink,
  oneTap,
  organization,
  twoFactor,
} from "better-auth/plugins";
import { Pool } from "pg";

const dryRun = process.argv.includes("--dry");

const auth = betterAuth({
  database: new Pool({
    connectionString: process.env.AUTH_DATABASE_URL ?? "",
  }),
  baseURL: process.env.BETTER_AUTH_URL,
  secret: process.env.BETTER_AUTH_SECRET,
  emailAndPassword: {
    enabled: true,
    requireEmailVerification: true,
  },
  emailVerification: {
    sendOnSignUp: true,
  },
  account: {
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
    organization({
      allowUserToCreateOrganization: true,
    }),
    magicLink({
      sendMagicLink: () => Promise.resolve(),
    }),
    passkey({
      rpID: new URL(process.env.BETTER_AUTH_URL ?? "http://localhost:3000")
        .hostname,
      rpName: "Strait",
      origin: process.env.BETTER_AUTH_URL ?? "http://localhost:3000",
    }),
    oneTap(),
    twoFactor(),
    jwt({
      jwks: {
        keyPairConfig: { alg: "RS256" },
        remoteUrl: `${process.env.BETTER_AUTH_URL ?? "http://localhost:3000"}/api/auth/jwks`,
      },
      jwt: {
        issuer: process.env.OIDC_ISSUER,
        audience: process.env.OIDC_AUDIENCE,
      },
    }),
    oauthProvider({
      loginPage: "/login",
      consentPage: "/oauth/consent",
      validAudiences: process.env.OIDC_AUDIENCE
        ? [process.env.OIDC_AUDIENCE]
        : undefined,
      allowDynamicClientRegistration: true,
      // allowUnauthenticatedClientRegistration is intentionally omitted (default: false).
      accessTokenExpiresIn: 900,
      refreshTokenExpiresIn: 2_592_000,
      codeExpiresIn: 600,
    }),
  ],
  user: {
    changeEmail: {
      enabled: true,
    },
    additionalFields: {
      defaultOrganizationId: {
        type: "string",
        required: false,
        input: false,
      },
      activeProjectId: {
        type: "string",
        required: false,
        input: false,
      },
    },
  },
});

async function migrate() {
  console.log("Checking for pending Better Auth migrations...\n");

  const { toBeCreated, toBeAdded, runMigrations } = await getMigrations(
    auth.options
  );

  if (toBeCreated.length === 0 && toBeAdded.length === 0) {
    console.log("No pending migrations. Database schema is up to date.");
    process.exit(0);
  }

  if (toBeCreated.length > 0) {
    console.log("Tables to create:");
    for (const table of toBeCreated) {
      console.log(`  + ${table.table}`);
      for (const field of Object.keys(table.fields)) {
        console.log(`      - ${field}`);
      }
    }
    console.log();
  }

  if (toBeAdded.length > 0) {
    console.log("Columns to add:");
    for (const col of toBeAdded) {
      console.log(
        `  + ${col.table}.${col.fields ? Object.keys(col.fields).join(", ") : "unknown"}`
      );
    }
    console.log();
  }

  if (dryRun) {
    console.log("Dry run complete. No changes applied.");
    process.exit(0);
  }

  console.log("Applying migrations...");
  await runMigrations();
  console.log("Migrations applied successfully.");
  process.exit(0);
}

migrate().catch((err) => {
  console.error("Migration failed:", err);
  process.exit(1);
});
