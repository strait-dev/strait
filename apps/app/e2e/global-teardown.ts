import pg from "pg";

export default async function globalTeardown() {
  const email = process.env.E2E_USER_EMAIL;
  const authDbUrl = process.env.AUTH_DATABASE_URL;

  if (!(email && authDbUrl)) {
    return;
  }

  const pool = new pg.Pool({ connectionString: authDbUrl });

  try {
    // Get the user ID first
    const userResult = await pool.query(
      `SELECT "id" FROM "user" WHERE "email" = $1`,
      [email]
    );

    if (userResult.rows.length === 0) {
      return;
    }

    const userId = userResult.rows[0].id;

    // Delete in dependency order
    await pool.query(`DELETE FROM "session" WHERE "userId" = $1`, [userId]);
    await pool.query(`DELETE FROM "account" WHERE "userId" = $1`, [userId]);

    // Delete org memberships and orgs created by this user
    const orgs = await pool.query(
      `SELECT "organizationId" FROM "member" WHERE "userId" = $1`,
      [userId]
    );
    await pool.query(`DELETE FROM "member" WHERE "userId" = $1`, [userId]);

    for (const row of orgs.rows) {
      // Only delete orgs that have no other members
      const memberCount = await pool.query(
        `SELECT COUNT(*) FROM "member" WHERE "organizationId" = $1`,
        [row.organizationId]
      );
      if (Number.parseInt(memberCount.rows[0].count, 10) === 0) {
        await pool.query(
          `DELETE FROM "invitation" WHERE "organizationId" = $1`,
          [row.organizationId]
        );
        await pool.query(`DELETE FROM "organization" WHERE "id" = $1`, [
          row.organizationId,
        ]);
      }
    }

    await pool.query(`DELETE FROM "user" WHERE "id" = $1`, [userId]);
  } finally {
    await pool.end();
  }
}
