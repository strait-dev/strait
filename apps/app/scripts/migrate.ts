/**
 * Programmatic database migration for Better Auth.
 *
 * Usage:
 *   bun run db:migrate          # apply pending migrations
 *   bun run db:migrate --dry    # show pending migrations without applying
 *
 * This script uses Better Auth's built-in migration system which is compatible
 * with the Kysely adapter. It avoids the @better-auth/cli which has module
 * resolution issues in monorepo setups.
 */
import { getMigrations } from "better-auth/db/migration";
import { auth } from "../src/lib/auth.server";

const dryRun = process.argv.includes("--dry");

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
      console.log(`  + ${col.table}.${col.fields ? Object.keys(col.fields).join(", ") : "unknown"}`);
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
