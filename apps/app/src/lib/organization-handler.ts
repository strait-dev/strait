import {
  OrganizationDeleted,
  OrganizationVerificationCode,
} from "@strait/transactional";
import { createServerFn } from "@tanstack/react-start";
import { getRequestHeaders } from "@tanstack/react-start/server";
import { nanoid } from "nanoid";
import z from "zod/v4";
import { apiPath } from "@/lib/api-client.server";
import { getAuth, getAuthPool } from "@/lib/auth.server";
import { apiEffect, runWithSentryReport } from "@/lib/effect-api.server";
import { kvGet, kvGetDelete, kvSet, kvSetIfAbsent } from "@/lib/kv.server";
import { ensureProjectTable } from "@/lib/project-handler";
import { getResend } from "@/lib/resend.server";
import type {
  ResendOrganizationDeletionCodeResponseSchema,
  VerifyOrganizationDeletionResponseSchema,
} from "@/lib/schema";
import {
  DeleteLastOrganizationWithTokenSchema,
  DeleteOrganizationWithTokenSchema,
  RequestOrganizationDeletionSchema,
  ResendOrganizationDeletionCodeSchema,
  VerifyOrganizationDeletionSchema,
} from "@/lib/schema";
import { authMiddleware } from "@/middlewares/auth";

type ProjectRow = {
  id: string;
};

type DeletionOperation = "delete";

const COOLDOWN_SECONDS = 60;
const CODE_TTL_SECONDS = 300;
const TOKEN_TTL_SECONDS = 300;

function deletionCodeKey(organizationId: string, userId: string): string {
  return `org-deletion:${organizationId}:${userId}`;
}

function deletionCooldownKey(organizationId: string, userId: string): string {
  return `org-deletion-cooldown:${organizationId}:${userId}`;
}

function deletionTokenKey(
  organizationId: string,
  userId: string,
  operation: DeletionOperation
): string {
  return `org-deletion-token:${operation}:${organizationId}:${userId}`;
}

function remainingCooldownSeconds(stored: string | null): number {
  if (!stored) {
    return 0;
  }
  const now = Date.now();
  const then = Number.parseInt(stored, 10);
  if (!Number.isFinite(then)) {
    return 0;
  }
  return Math.max(0, Math.ceil((then + COOLDOWN_SECONDS * 1000 - now) / 1000));
}

/**
 * Server function to create a new organization.
 * Used by onboarding flow — creates organization via Better Auth.
 * Returns serializable organization data.
 */
export const createOrganizationServerFn = createServerFn({ method: "POST" })
  .inputValidator((data: Record<string, unknown>) => data)
  .middleware([authMiddleware])
  .handler(async ({ data }) => {
    try {
      const headers = getRequestHeaders();

      const name = data.name as string;
      const slug =
        (data.slug as string | undefined) ||
        name
          .toLowerCase()
          .replace(/\s+/g, "-")
          .replace(/[^a-z0-9-]/g, "");
      const org = await (await getAuth()).api.createOrganization({
        body: {
          name,
          slug,
        },
        headers,
      });

      if (!org) {
        return null;
      }

      return {
        id: org.id,
        name: org.name,
        slug: org.slug ?? null,
        logo: org.logo ?? null,
        metadata: org.metadata ?? null,
        createdAt: org.createdAt,
      };
    } catch {
      return null;
    }
  });

/**
 * Get full organization from Better Auth.
 * @param {string} organizationId - The ID of the organization to get.
 * @returns {Promise<Organization>} A promise resolving to the organization data.
 */
const getFullOrganizationAuth = createServerFn({ method: "GET" })
  .inputValidator((data: { organizationId: string }) =>
    z.object({ organizationId: z.string() }).parse(data)
  )
  .handler(async ({ data }) => {
    try {
      const headers = getRequestHeaders();

      const org = await (await getAuth()).api.getFullOrganization({
        query: { organizationId: data.organizationId },
        headers,
      });

      if (!org) {
        return null;
      }

      return { ...org, members: org.members ?? [] };
    } catch {
      return null;
    }
  });

/**
 * Set active organization via Better Auth.
 * @param {string} organizationId - The ID of the organization to set as active.
 * @returns {Promise<Organization | null>} A promise resolving to the organization data.
 */
export const setActiveOrganizationAuth = createServerFn({ method: "POST" })
  .inputValidator((data: { organizationId: string }) =>
    z.object({ organizationId: z.string() }).parse(data)
  )
  .middleware([authMiddleware])
  .handler(async ({ data }) => {
    const headers = getRequestHeaders();
    await ensureProjectTable(getAuthPool());

    const auth = await getAuth();
    const result = await auth.api.setActiveOrganization({
      body: { organizationId: data.organizationId },
      headers,
    });

    if (!result) {
      throw new Error("Failed to set active organization");
    }

    const projectResult = await getAuthPool().query<ProjectRow>(
      `SELECT id
       FROM project
       WHERE organization_id = $1
       ORDER BY created_at ASC
       LIMIT 1`,
      [data.organizationId]
    );

    const activeProjectId = projectResult.rows[0]?.id ?? null;

    const session = await auth.api.getSession({ headers });
    if (!session?.user?.id) {
      throw new Error("Unauthorized");
    }

    await getAuthPool().query(
      `UPDATE "user"
       SET "defaultOrganizationId" = $1, "activeProjectId" = $2
       WHERE id = $3`,
      [data.organizationId, activeProjectId, session.user.id]
    );

    return result;
  });

/**
 * List organizations from Better Auth.
 * @returns {Promise<Organization[]>} A promise resolving to the organizations data.
 */
const listOrganizationsAuth = createServerFn({ method: "GET" }).handler(
  async () => {
    try {
      const headers = getRequestHeaders();

      const organizations = await (await getAuth()).api.listOrganizations({
        headers,
      });

      return organizations ?? [];
    } catch {
      return [];
    }
  }
);

async function listOrganizationProjectIds(
  organizationId: string
): Promise<string[]> {
  await ensureProjectTable(getAuthPool());
  const result = await getAuthPool().query<ProjectRow>(
    "SELECT id FROM project WHERE organization_id = $1 ORDER BY created_at ASC",
    [organizationId]
  );
  return result.rows.map((row) => row.id);
}

async function deleteBackendProjects(projectIds: string[]): Promise<void> {
  for (const projectId of projectIds) {
    try {
      await runWithSentryReport(
        apiEffect(apiPath`/v1/projects/${projectId}`, {
          method: "DELETE",
          projectId,
        })
      );
    } catch (error) {
      if (error instanceof Error && error.message.includes("(404)")) {
        continue;
      }
      throw error;
    }
  }
}

async function clearActiveProjectIds(projectIds: string[]): Promise<void> {
  if (projectIds.length === 0) {
    return;
  }
  await getAuthPool().query(
    `UPDATE "user"
     SET "activeProjectId" = NULL
     WHERE "activeProjectId" = ANY($1::text[])`,
    [projectIds]
  );
}

/**
 * Request organization deletion.
 * Sends verification code via email and implements rate limiting.
 */
export const requestOrganizationDeletionServerFn = createServerFn({
  method: "POST",
})
  .inputValidator((data: z.infer<typeof RequestOrganizationDeletionSchema>) =>
    RequestOrganizationDeletionSchema.parse(data)
  )
  .middleware([authMiddleware])
  .handler(async ({ context, data }) => {
    const { organizationId } = data;

    const cooldownKey = deletionCooldownKey(organizationId, context.user.id);
    const lastRequestTime = await kvGet(cooldownKey);
    const remainingTime = remainingCooldownSeconds(lastRequestTime);

    if (remainingTime > 0) {
      return {
        success: true,
        cooldownRemaining: remainingTime,
      };
    }

    if (data.checkCooldownOnly) {
      return {
        success: true,
        cooldownRemaining: 0,
      };
    }

    const organization = await getFullOrganizationAuth({
      data: { organizationId },
    });

    if (!organization) {
      throw new Error("Organization not found");
    }

    const SIX_DIGIT_CODE_LENGTH = 6;
    const now = Date.now();
    const cooldownAcquired = await kvSetIfAbsent(cooldownKey, now.toString(), {
      ex: COOLDOWN_SECONDS,
    });

    if (!cooldownAcquired) {
      return {
        success: true,
        cooldownRemaining: COOLDOWN_SECONDS,
      };
    }

    const verificationCode = nanoid(SIX_DIGIT_CODE_LENGTH);

    await kvSet(
      deletionCodeKey(organizationId, context.user.id),
      verificationCode,
      {
        ex: CODE_TTL_SECONDS,
      }
    );

    await getResend().emails.send({
      from: "Strait <noreply@strait.dev>",
      to: context.user.email,
      subject: `Verification code for organization deletion of ${organization.name}`,
      react: OrganizationVerificationCode({
        name: context.user.name,
        organizationName: organization.name,
        verificationCode,
      }),
    });

    return {
      success: true,
      cooldownRemaining: COOLDOWN_SECONDS,
    };
  });

/**
 * Server function to verify organization deletion code.
 * Returns verification token on success.
 */
export const verifyOrganizationDeletionServerFn = createServerFn({
  method: "POST",
})
  .inputValidator((data: z.infer<typeof VerifyOrganizationDeletionSchema>) =>
    VerifyOrganizationDeletionSchema.parse(data)
  )
  .middleware([authMiddleware])
  .handler(async ({ context, data }) => {
    const key = deletionCodeKey(data.organizationId, context.user.id);
    const storedCode = await kvGetDelete(key);

    const storedCodeStr = storedCode ? storedCode.toString() : null;
    const inputCodeStr = data.verificationCode;
    if (!storedCodeStr || storedCodeStr !== inputCodeStr) {
      return {
        success: false,
        message: "Verification code invalid or expired",
      } satisfies z.infer<typeof VerifyOrganizationDeletionResponseSchema>;
    }

    const ONE_TIME_TOKEN_LENGTH = 15;

    const verificationToken = `${context.user.id}-${Date.now()}-${nanoid(ONE_TIME_TOKEN_LENGTH)}`;

    const tokenKey = deletionTokenKey(
      data.organizationId,
      context.user.id,
      data.operation
    );
    await kvSet(tokenKey, verificationToken, { ex: TOKEN_TTL_SECONDS });

    return {
      success: true,
      verificationToken,
    } satisfies z.infer<typeof VerifyOrganizationDeletionResponseSchema>;
  });

/**
 * Server function to delete organization with verification token.
 * Handles organization deletion and switches to next organization.
 */
export const deleteOrganizationWithTokenServerFn = createServerFn({
  method: "POST",
})
  .inputValidator((data: z.infer<typeof DeleteOrganizationWithTokenSchema>) =>
    DeleteOrganizationWithTokenSchema.parse(data)
  )
  .middleware([authMiddleware])
  .handler(async ({ context, data }) => {
    const { organizationId, verificationToken, nextOrganizationId } = data;

    const tokenKey = deletionTokenKey(
      organizationId,
      context.user.id,
      "delete"
    );
    const storedToken = await kvGetDelete(tokenKey);

    if (!storedToken || storedToken !== verificationToken) {
      return {
        success: false,
        message: "Verification token invalid or expired",
      };
    }

    const organization = await getFullOrganizationAuth({
      data: { organizationId },
    });

    if (!organization) {
      throw new Error("Organization not found");
    }

    const headers = getRequestHeaders();
    const organizations = await listOrganizationsAuth();
    const otherOrganizations = organizations.filter(
      (org: { id: string }) => org.id !== organizationId
    );

    if (otherOrganizations.length === 0) {
      throw new Error("Use the last organization deletion flow");
    }

    if (
      !(
        nextOrganizationId &&
        otherOrganizations.some(
          (org: { id: string }) => org.id === nextOrganizationId
        )
      )
    ) {
      throw new Error("A valid next organization is required");
    }

    // Check if this is the user's default organization
    const session = await (await getAuth()).api.getSession({ headers });
    const defaultOrgId = (session?.user as Record<string, unknown> | undefined)
      ?.defaultOrganizationId as string | undefined;
    const isDefaultOrganization = defaultOrgId === organizationId;
    const projectIds = await listOrganizationProjectIds(organizationId);

    await deleteBackendProjects(projectIds);
    await clearActiveProjectIds(projectIds);

    if (isDefaultOrganization) {
      await setActiveOrganizationAuth({
        data: { organizationId: nextOrganizationId },
      });
    }

    // Delete the organization via Better Auth
    await (await getAuth()).api.deleteOrganization({
      body: { organizationId },
      headers,
    });

    await getResend().emails.send({
      from: "Strait <hello@usestrait.com>",
      to: context.user.email,
      subject: "Organization deleted successfully",
      react: OrganizationDeleted({
        name: context.user.name,
      }),
    });

    return {
      success: true,
      organizationId,
      deleted: true,
      user: context.user,
    };
  });

/**
 * Server function to resend organization deletion code.
 * Implements rate limiting and sends new verification code.
 */
export const resendOrganizationDeletionCodeServerFn = createServerFn({
  method: "POST",
})
  .inputValidator(
    (data: z.infer<typeof ResendOrganizationDeletionCodeSchema>) =>
      ResendOrganizationDeletionCodeSchema.parse(data)
  )
  .middleware([authMiddleware])
  .handler(async ({ context, data }) => {
    const { organizationId } = data;

    const cooldownKey = deletionCooldownKey(organizationId, context.user.id);
    const lastRequestTime = await kvGet(cooldownKey);
    const remainingTime = remainingCooldownSeconds(lastRequestTime);

    if (remainingTime > 0) {
      return {
        success: false,
        message: "Please wait 60 seconds before requesting a new code",
        cooldownRemaining: remainingTime,
      } satisfies z.infer<typeof ResendOrganizationDeletionCodeResponseSchema>;
    }

    const organization = await getFullOrganizationAuth({
      data: { organizationId },
    });

    if (!organization) {
      throw new Error("Organization not found");
    }

    const SIX_DIGIT_CODE_LENGTH = 6;
    const now = Date.now();
    const cooldownAcquired = await kvSetIfAbsent(cooldownKey, now.toString(), {
      ex: COOLDOWN_SECONDS,
    });

    if (!cooldownAcquired) {
      return {
        success: false,
        message: "Please wait 60 seconds before requesting a new code",
        cooldownRemaining: COOLDOWN_SECONDS,
      } satisfies z.infer<typeof ResendOrganizationDeletionCodeResponseSchema>;
    }

    const verificationCode = nanoid(SIX_DIGIT_CODE_LENGTH);

    await kvSet(
      deletionCodeKey(organizationId, context.user.id),
      verificationCode,
      {
        ex: CODE_TTL_SECONDS,
      }
    );

    await getResend().emails.send({
      from: "Strait <hello@usestrait.com>",
      to: context.user.email,
      subject: `Verification code for organization deletion of ${organization.name}`,
      react: OrganizationVerificationCode({
        name: context.user.name,
        organizationName: organization.name,
        verificationCode,
      }),
    });

    return {
      success: true,
      cooldownRemaining: COOLDOWN_SECONDS,
    } satisfies z.infer<typeof ResendOrganizationDeletionCodeResponseSchema>;
  });

/**
 * Server function to delete the last organization with verification token.
 * Handles complete organization deletion with verification token.
 */
export const deleteLastOrganizationWithTokenServerFn = createServerFn({
  method: "POST",
})
  .inputValidator(
    (data: z.infer<typeof DeleteLastOrganizationWithTokenSchema>) =>
      DeleteLastOrganizationWithTokenSchema.parse(data)
  )
  .middleware([authMiddleware])
  .handler(async ({ context, data }) => {
    const { organizationId, verificationToken } = data;

    const tokenKey = deletionTokenKey(
      organizationId,
      context.user.id,
      "delete"
    );
    const storedToken = await kvGetDelete(tokenKey);

    if (!storedToken || storedToken !== verificationToken) {
      return {
        success: false,
        message: "Verification token invalid or expired",
      };
    }

    const organization = await getFullOrganizationAuth({
      data: { organizationId },
    });

    if (!organization) {
      throw new Error("Organization not found");
    }

    const headers = getRequestHeaders();
    const organizations = await listOrganizationsAuth();
    if (
      organizations.some((org: { id: string }) => org.id === organizationId) &&
      organizations.length > 1
    ) {
      throw new Error("Use the organization switch deletion flow");
    }

    const projectIds = await listOrganizationProjectIds(organizationId);
    await deleteBackendProjects(projectIds);
    await clearActiveProjectIds(projectIds);

    const auth = await getAuth();
    const session = await auth.api.getSession({ headers });
    if (!session?.user?.id) {
      throw new Error("Unauthorized");
    }

    await getAuthPool().query(
      `UPDATE "user"
       SET "defaultOrganizationId" = NULL, "activeProjectId" = NULL
       WHERE id = $1`,
      [session.user.id]
    );

    // Delete the organization via Better Auth
    await auth.api.deleteOrganization({
      body: { organizationId },
      headers,
    });

    try {
      await getResend().emails.send({
        from: "Strait <hello@usestrait.com>",
        to: context.user.email,
        subject: "Organization deleted successfully",
        react: OrganizationDeleted({
          name: context.user.name,
        }),
      });

      return {
        success: true,
        message: "Organization deleted successfully",
        organizationId,
        userOnboardingReset: true,
      };
    } catch {
      throw new Error("Error deleting organization");
    }
  });
