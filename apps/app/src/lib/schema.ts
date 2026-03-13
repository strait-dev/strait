import { isValidPhoneNumber } from "react-phone-number-input";
import * as z from "zod";

const DEFAULT_MIN_CODE_LENGTH = 6;
const DEFAULT_MAX_COMPANY_LENGTH = 100;
const DEFAULT_MAX_PRIORITY_LENGTH = 8;
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

export const DeleteOrganizationSchema = z.object({
  confirm: z
    .boolean()
    .refine((val) => val === true, { message: "Confirm deletion" }),
  word: z
    .string()
    .min(1, { message: "Type the word correctly" })
    .refine((val) => val.toLowerCase() === "delete", {
      message: "Type exactly the word 'delete'",
    }),
});

export const VerifyCodeDeletionSchema = z.object({
  verificationCode: z
    .string()
    .min(DEFAULT_MIN_CODE_LENGTH, {
      message: "Code must be at least 6 characters",
    })
    .max(DEFAULT_MIN_CODE_LENGTH, {
      message: "Code must be at most 6 characters",
    }),
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

// Step 1: Business Needs
const businessNeedsSchema = z.object({
  businessNeeds: z
    .array(z.string())
    .min(1, "Please select at least one business priority")
    .max(DEFAULT_MAX_PRIORITY_LENGTH, "Please select up to 8 priorities"),
});

// Step 2: Company Information
const companyInfoSchema = z.object({
  companyName: z
    .string()
    .min(2, "Company name must be at least 2 characters")
    .max(
      DEFAULT_MAX_COMPANY_LENGTH,
      "Company name must be less than 100 characters"
    ),
  companyPhone: z
    .string()
    .optional()
    .refine(
      (val) => {
        if (!val || val === "") {
          return true;
        }
        return isValidPhoneNumber(val);
      },
      { message: "Please enter a valid phone number" }
    ),
  industry: z.string().min(1, "Please select your industry"),
  companySize: z.string().min(1, "Please select your company size"),
  businessType: z.string().min(1, "Please select your business type"),
  annualRevenue: z
    .enum([
      "under_50k",
      "50k_to_100k",
      "100k_to_500k",
      "500k_to_1m",
      "1m_to_5m",
      "5m_to_10m",
      "10m_to_50m",
      "over_50m",
      "prefer_not_to_say",
    ])
    .optional(),
  country: z.string().min(1, "Please select your country"),
  website: z
    .string()
    .optional()
    .refine(
      (val) => {
        if (!val || val === "") {
          return true;
        }
        try {
          new URL(val);
          return true;
        } catch {
          return false;
        }
      },
      { message: "Please enter a valid URL" }
    ),
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
  ...businessNeedsSchema.shape,
  ...companyInfoSchema.shape,
});

export type OnboardingFormData = z.infer<typeof onboardingSchema>;
