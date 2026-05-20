import * as z from "zod";

export const FeedbackFormSchema = z.object({
  email: z.email("Invalid email"),
  subject: z
    .string("You must choose a subject")
    .min(1, { message: "You must choose a subject" }),
  message: z
    .string("Message is required")
    .min(10, { message: "Message must be at least 10 characters" }),
});

export const SupportFormSchema = z.object({
  email: z.email("Invalid email"),
  subject: z.string().min(1, "Select a subject"),
  priority: z.enum(["low", "medium", "high"]).default("medium"),
  environment: z
    .enum(["production", "development", "staging"])
    .default("development"),
  message: z.string().min(10, "Message must be at least 10 characters"),
  steps_to_reproduce: z
    .string()
    .min(10, "Describe the steps to reproduce the problem"),
  expected_result: z.string().min(10, "Describe the expected result"),
  actual_result: z.string().min(10, "Describe the actual result"),
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
