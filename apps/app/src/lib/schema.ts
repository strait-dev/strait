import * as z from "zod";

const DEFAULT_MAX_WORKSPACE_LENGTH = 100;
const DEFAULT_MAX_USE_CASE_LENGTH = 8;
const DEFAULT_MAX_DESCRIPTION_LENGTH = 500;

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

// Step 1: Use Cases
const useCasesSchema = z.object({
  useCases: z
    .array(z.string())
    .min(1, "Please select at least one use case")
    .max(DEFAULT_MAX_USE_CASE_LENGTH, "Please select up to 8 use cases"),
});

// Step 2: Workspace Setup
const workspaceSetupSchema = z.object({
  workspaceName: z
    .string()
    .min(2, "Workspace name must be at least 2 characters")
    .max(
      DEFAULT_MAX_WORKSPACE_LENGTH,
      "Workspace name must be less than 100 characters"
    ),
  teamSize: z.string().min(1, "Please select your team size"),
  environment: z.string().min(1, "Please select your primary environment"),
  primaryGoals: z
    .string()
    .optional()
    .refine(
      (val) => {
        if (!val) {
          return true;
        }
        return val.length <= DEFAULT_MAX_DESCRIPTION_LENGTH;
      },
      { message: "Goals description must be less than 500 characters" }
    ),
});

// Combined schema for the entire onboarding form
export const onboardingSchema = z.object({
  ...useCasesSchema.shape,
  ...workspaceSetupSchema.shape,
});

export type OnboardingFormData = z.infer<typeof onboardingSchema>;
