/**
 * Programmatic database migration for Better Auth.
 *
 * Usage:
 *   bun run db:migrate          # apply pending migrations
 *   bun run db:migrate --dry    # show pending migrations without applying
 */
import { migrateAuthDatabase } from "./lib/local-bootstrap";

const dryRun = process.argv.includes("--dry");

async function migrate() {
  console.log("Checking for pending Better Auth migrations...\n");
  await migrateAuthDatabase(dryRun);
  if (dryRun) {
    console.log("Dry run complete. No changes applied.");
    process.exit(0);
  }
  console.log("Migrations applied successfully.");
  process.exit(0);
}

migrate().catch((err) => {
  console.error("Migration failed:", err);
  process.exit(1);
});
