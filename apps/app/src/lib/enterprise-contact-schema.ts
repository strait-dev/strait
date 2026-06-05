import { z } from "zod/v4";

export const TEAM_SIZES = [
  "1-10",
  "11-50",
  "51-200",
  "201-500",
  "500+",
] as const;

export const USE_CASES = [
  "Roadmap security / compliance",
  "High volume workloads",
  "Roadmap isolation requirements",
  "Roadmap residency requirements",
  "Other",
] as const;

export const MONTHLY_SPEND_RANGES = [
  "Under $500/mo",
  "$500 - $1,500/mo",
  "$1,500 - $4,000/mo",
  "$4,000 - $8,000/mo",
  "Over $8,000/mo",
] as const;

const isTeamSize = (value: string) =>
  TEAM_SIZES.includes(value as (typeof TEAM_SIZES)[number]);

const isUseCase = (value: string) =>
  USE_CASES.includes(value as (typeof USE_CASES)[number]);

const isMonthlySpendRange = (value: string) =>
  MONTHLY_SPEND_RANGES.includes(value as (typeof MONTHLY_SPEND_RANGES)[number]);

export const enterpriseContactSchema = z.object({
  name: z
    .string()
    .min(1, "Name is required")
    .max(120, "Name must be 120 characters or less"),
  email: z.email("Must be a valid email address").max(254, "Email is too long"),
  company: z
    .string()
    .min(1, "Company name is required")
    .max(120, "Company name must be 120 characters or less"),
  teamSize: z
    .string()
    .min(1, "Team size is required")
    .max(20, "Team size is too long")
    .refine(isTeamSize, "Select a valid team size"),
  useCase: z
    .string()
    .max(80, "Use case is too long")
    .refine((value) => value === "" || isUseCase(value), {
      message: "Select a valid use case",
    }),
  expectedSpend: z
    .string()
    .max(80, "Expected spend is too long")
    .refine((value) => value === "" || isMonthlySpendRange(value), {
      message: "Select a valid spend range",
    }),
  message: z
    .string()
    .min(10, "Message must be at least 10 characters")
    .max(4000, "Message must be 4000 characters or less"),
});
