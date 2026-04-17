import {
  OrganizationDeleted,
  OrganizationPurged,
  OrganizationVerificationCode,
} from "@strait/transactional";
import { createServerFn } from "@tanstack/react-start";
import { getRequestHeaders } from "@tanstack/react-start/server";
import { nanoid } from "nanoid";
import z from "zod/v4";
import { getAuth } from "@/lib/auth.server";
import { kvGet, kvSet } from "@/lib/kv.server";
import { getResend } from "@/lib/resend.server";
import type {
  ResendOrganizationDeletionCodeResponseSchema,
  VerifyOrganizationDeletionResponseSchema,
} from "@/lib/schema";
import {
  DeleteLastOrganizationWithTokenSchema,
  DeleteOrganizationWithTokenSchema,
  PurgeOrganizationWithTokenSchema,
  RequestOrganizationDeletionSchema,
  ResendOrganizationDeletionCodeSchema,
  VerifyOrganizationDeletionSchema,
} from "@/lib/schema";
import { authMiddleware } from "@/middlewares/auth";

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
  .handler(async ({ data }) => {
    try {
      const headers = getRequestHeaders();

      const result = await (await getAuth()).api.setActiveOrganization({
        body: { organizationId: data.organizationId },
        headers,
      });

      // Also update the user's defaultOrganizationId
      await (await getAuth()).api.updateUser({
        body: { defaultOrganizationId: data.organizationId },
        headers,
      });

      return result ?? null;
    } catch {
      return null;
    }
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

    const cooldownKey = `org-deletion-cooldown:${organizationId}:${context.user.id}`;
    const lastRequestTime = await kvGet(cooldownKey);
    const now = Date.now();

    const COOLDOWN_TIME = 60_000;
    const THOUSAND = 1000;

    if (
      lastRequestTime &&
      now - Number.parseInt(lastRequestTime as string, 10) < COOLDOWN_TIME
    ) {
      const remainingTime = Math.ceil(
        (Number.parseInt(lastRequestTime as string, 10) + COOLDOWN_TIME - now) /
          THOUSAND
      );

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

    const key = `org-deletion:${organizationId}:${context.user.id}`;

    const SIX_DIGIT_CODE_LENGTH = 6;
    const FIVE_MINUTES_S = 300;
    const COOLDOWN_TIME_S = 60;

    const verificationCode = nanoid(SIX_DIGIT_CODE_LENGTH);

    await kvSet(key, verificationCode, { ex: FIVE_MINUTES_S });
    await kvSet(cooldownKey, now.toString(), { ex: COOLDOWN_TIME_S });

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
      cooldownRemaining: COOLDOWN_TIME,
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
    const key = `org-deletion:${data.organizationId}:${context.user.id}`;
    const storedCode = await kvGet(key);

    const storedCodeStr = storedCode ? storedCode.toString() : null;
    const inputCodeStr = data.verificationCode;
    if (!storedCodeStr || storedCodeStr !== inputCodeStr) {
      return {
        success: false,
        message: "Verification code invalid or expired",
      } satisfies z.infer<typeof VerifyOrganizationDeletionResponseSchema>;
    }

    const ONE_TIME_TOKEN_LENGTH = 15;
    const FIVE_MINUTES_S = 300;

    const verificationToken = `${context.user.id}-${Date.now()}-${nanoid(ONE_TIME_TOKEN_LENGTH)}`;

    const tokenKey = `org-deletion-token:${data.organizationId}:${context.user.id}`;
    await kvSet(tokenKey, verificationToken, { ex: FIVE_MINUTES_S });

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

    const tokenKey = `org-deletion-token:${organizationId}:${context.user.id}`;
    const storedToken = await kvGet(tokenKey);

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

    // Check if this is the user's default organization
    const session = await (await getAuth()).api.getSession({ headers });
    const defaultOrgId = (session?.user as Record<string, unknown> | undefined)
      ?.defaultOrganizationId as string | undefined;
    const isDefaultOrganization = defaultOrgId === organizationId;

    if (isDefaultOrganization) {
      if (nextOrganizationId) {
        await setActiveOrganizationAuth({
          data: { organizationId: nextOrganizationId },
        });
      }

      try {
        await (await getAuth()).api.updateUser({
          body: { activeProjectId: null },
          headers,
        });
      } catch {
        // Best-effort cleanup — projects are cascade-deleted with the org
      }
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
 * Server function to purge organization data with verification token.
 * Clears all organization data but keeps the organization structure.
 */
export const purgeOrganizationWithTokenServerFn = createServerFn({
  method: "POST",
})
  .inputValidator((data: z.infer<typeof PurgeOrganizationWithTokenSchema>) =>
    PurgeOrganizationWithTokenSchema.parse(data)
  )
  .middleware([authMiddleware])
  .handler(async ({ context, data }) => {
    const { organizationId, verificationToken } = data;

    const tokenKey = `org-deletion-token:${organizationId}:${context.user.id}`;
    const storedToken = await kvGet(tokenKey);

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

    const membership = organization.members.find(
      (member: any) => member.userId === context.user.id
    );

    if (!(membership && ["owner", "admin"].includes(membership.role))) {
      throw new Error(
        "You do not have permission to purge the data of this organization"
      );
    }

    const organizations = await listOrganizationsAuth();

    if (organizations && organizations.length > 1) {
      throw new Error(
        "This action can only be used when there is only one organization"
      );
    }

    try {
      await getResend().emails.send({
        from: "Strait <hello@usestrait.com>",
        to: context.user.email,
        subject: "Organization data purged successfully",
        react: OrganizationPurged({
          name: context.user.name,
          organizationName: organization.name,
        }),
      });

      return {
        success: true,
        message: "Organization data purged successfully",
        organizationId,
      };
    } catch {
      throw new Error("Error purging organization data");
    }
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

    const cooldownKey = `org-deletion-cooldown:${organizationId}:${context.user.id}`;
    const lastRequestTime = await kvGet(cooldownKey);
    const now = Date.now();

    const COOLDOWN_TIME = 60_000;
    const THOUSAND = 1000;

    if (
      lastRequestTime &&
      now - Number.parseInt(lastRequestTime as string, 10) < COOLDOWN_TIME
    ) {
      const remainingTime = Math.ceil(
        (Number.parseInt(lastRequestTime as string, 10) + COOLDOWN_TIME - now) /
          THOUSAND
      );

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

    const key = `org-deletion:${organizationId}:${context.user.id}`;

    const SIX_DIGIT_CODE_LENGTH = 6;
    const FIVE_MINUTES_S = 300;
    const COOLDOWN_TIME_S = 60;

    const verificationCode = nanoid(SIX_DIGIT_CODE_LENGTH);

    await kvSet(key, verificationCode, { ex: FIVE_MINUTES_S });
    await kvSet(cooldownKey, now.toString(), { ex: COOLDOWN_TIME_S });

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
      cooldownRemaining: COOLDOWN_TIME,
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

    const tokenKey = `org-deletion-token:${organizationId}:${context.user.id}`;
    const storedToken = await kvGet(tokenKey);

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

    // Delete the organization via Better Auth
    await (await getAuth()).api.deleteOrganization({
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
