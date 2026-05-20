import * as z from "zod";

const FEEDBACK_SUBJECTS = [
  "bug",
  "feedback",
  "featureRequest",
  "question",
  "other",
] as const;

const SUPPORT_SUBJECTS = ["technical", "billing", "account", "other"] as const;

export const FeedbackFormSchema = z.object({
  email: z.email("Invalid email").max(254, "Email is too long"),
  subject: z
    .string("You must choose a subject")
    .min(1, { message: "You must choose a subject" })
    .max(32, { message: "Subject is too long" })
    .refine(
      (value) =>
        FEEDBACK_SUBJECTS.includes(value as (typeof FEEDBACK_SUBJECTS)[number]),
      { message: "You must choose a valid subject" }
    ),
  message: z
    .string("Message is required")
    .min(10, { message: "Message must be at least 10 characters" })
    .max(4000, { message: "Message must be 4000 characters or less" }),
});

export const SupportFormSchema = z.object({
  email: z.email("Invalid email").max(254, "Email is too long"),
  subject: z
    .string()
    .min(1, "Select a subject")
    .max(32, "Subject is too long")
    .refine(
      (value) =>
        SUPPORT_SUBJECTS.includes(value as (typeof SUPPORT_SUBJECTS)[number]),
      { message: "Select a valid subject" }
    ),
  priority: z.enum(["low", "medium", "high"]).default("medium"),
  environment: z
    .enum(["production", "development", "staging"])
    .default("development"),
  message: z
    .string()
    .min(10, "Message must be at least 10 characters")
    .max(4000, "Message must be 4000 characters or less"),
  steps_to_reproduce: z
    .string()
    .min(10, "Describe the steps to reproduce the problem")
    .max(4000, "Steps must be 4000 characters or less"),
  expected_result: z
    .string()
    .min(10, "Describe the expected result")
    .max(4000, "Expected result must be 4000 characters or less"),
  actual_result: z
    .string()
    .min(10, "Describe the actual result")
    .max(4000, "Actual result must be 4000 characters or less"),
});

export const RequestOrganizationDeletionSchema = z.object({
  organizationId: z.string({ message: "Invalid organization ID" }),
  checkCooldownOnly: z.boolean().optional(),
});

export const VerifyOrganizationDeletionSchema = z.object({
  organizationId: z.string({ message: "Invalid organization ID" }),
  verificationCode: z.string({ message: "Invalid verification code" }),
  operation: z.enum(["delete", "purge"]).default("delete"),
});

export const VerifyOrganizationDeletionResponseSchema = z.object({
  success: z.boolean(),
  message: z.string().optional(),
  verificationToken: z.string().optional(),
});

export const DeleteOrganizationWithTokenSchema = z.object({
  organizationId: z.string({ message: "Invalid organization ID" }),
  verificationToken: z.string({ message: "Invalid verification token" }),
  nextOrganizationId: z.string({
    message: "Invalid next organization ID",
  }),
});

export const ResendOrganizationDeletionCodeSchema = z.object({
  organizationId: z.string({ message: "Invalid organization ID" }),
});

export const ResendOrganizationDeletionCodeResponseSchema = z.object({
  success: z.boolean(),
  message: z.string().optional(),
  cooldownRemaining: z.number().optional(),
});

export const PurgeOrganizationWithTokenSchema = z.object({
  organizationId: z.string({ message: "Invalid organization ID" }),
  verificationToken: z.string({ message: "Invalid verification token" }),
});

export const DeleteLastOrganizationWithTokenSchema = z.object({
  organizationId: z.string({ message: "Invalid organization ID" }),
  verificationToken: z.string({ message: "Invalid verification token" }),
});
