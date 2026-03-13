import { nanoid } from "nanoid";
import type { OnboardingFormData } from "../lib/schema.ts";
import { MAX_SLUG_LENGTH, ORGANIZATION_SLUG_LENGTH } from "./constants.ts";

/**
 * Regex for removing trailing hyphens from slugs
 */
const TRAILING_HYPHENS_REGEX = /-+$/;

/**
 * Maps onboarding form company size to database employees_size enum
 */
const mapCompanySizeToEmployees = (size: string | undefined) => {
  switch (size) {
    case "solo":
      return "less_than_5" as const;
    case "small":
      return "five_to_ten" as const;
    case "medium":
      return "ten_to_twenty" as const;
    case "large":
      return "twenty_to_fifty" as const;
    case "enterprise":
      return "more_than_one_hundred" as const;
    default:
      return "five_to_ten" as const;
  }
};

/**
 * Maps onboarding form industry to database activity enum
 */
const mapIndustryToActivity = (industry: string | undefined) => {
  switch (industry) {
    case "retail":
      return "apparel_footwear" as const;
    case "technology":
      return "computers_internet" as const;
    case "healthcare":
      return "health_medicine" as const;
    case "food":
    case "food-beverage":
      return "food_beverage" as const;
    case "beauty":
    case "health-beauty":
      return "beauty_pharmacy" as const;
    case "service":
    case "services":
      return "financial_services" as const;
    case "manufacturing":
      return "machinery" as const;
    case "electronics":
      return "electronics" as const;
    case "automotive":
      return "vehicles" as const;
    case "fashion":
      return "apparel_footwear" as const;
    case "home-garden":
      return "home_decor" as const;
    case "sports-outdoors":
      return "sporting" as const;
    default:
      return "other" as const;
  }
};

/**
 * Maps onboarding form industry to database segment enum
 */
const mapIndustryToSegment = (industry: string | undefined) => {
  switch (industry) {
    case "retail":
    case "food":
    case "food-beverage":
    case "beauty":
    case "health-beauty":
    case "fashion":
    case "home-garden":
    case "sports-outdoors":
    case "automotive":
      return "commercial" as const;
    case "technology":
    case "electronics":
    case "wholesale":
      return "ecommerce" as const;
    case "manufacturing":
      return "industrial" as const;
    case "service":
    case "services":
    case "healthcare":
      return "service" as const;
    default:
      return "commercial" as const;
  }
};

/**
 * Maps onboarding form company size to database size enum
 */
const mapCompanySizeToSize = (size: string | undefined) => {
  switch (size) {
    case "solo":
      return "micro" as const;
    case "small":
      return "small" as const;
    case "medium":
      return "medium" as const;
    case "large":
    case "enterprise":
      return "large" as const;
    default:
      return "small" as const;
  }
};

/**
 * Generates a URL-friendly slug from a company name
 */
const generateSlug = (name: string): string => {
  return name
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-") // Replace non-alphanumeric with hyphens
    .replace(/^-+|-+$/g, "") // Remove leading/trailing hyphens
    .substring(0, MAX_SLUG_LENGTH) // Limit length to reasonable size
    .replace(TRAILING_HYPHENS_REGEX, ""); // Remove trailing hyphens again
};

/**
 * Transforms onboarding form data into organization creation data
 */
export const transformOnboardingToOrgData = (
  data: OnboardingFormData,
  userId: string
) => {
  const companyName = data.companyName || "My Company";
  const baseSlug = generateSlug(companyName);

  // Use company name as base with unique suffix, or fallback to generic org
  const slug = baseSlug
    ? `${baseSlug}-${nanoid(ORGANIZATION_SLUG_LENGTH)}`
    : `org-${nanoid(ORGANIZATION_SLUG_LENGTH)}`;

  return {
    id: nanoid(),
    name: companyName,
    slug,
    logo: "",
    metadata: "",
    email: "",
    phone: data.companyPhone || "",
    description: data.businessNeeds?.join(", ") || "",
    status: "active" as const,
    website: data.website || "",
    entity_type: "company" as const,
    tax_id: "",
    business_name: data.companyName || "",
    business_registration: "",
    industry_code: "",
    activity: mapIndustryToActivity(data.industry),
    segment: mapIndustryToSegment(data.industry),
    size: mapCompanySizeToSize(data.companySize),
    employees_size: mapCompanySizeToEmployees(data.companySize),
    address: "",
    postal_code: "",
    city: "",
    state: "",
    house_number: "",
    neighborhood: "",
    complement: "",
    country: data.country || "",
    currency_code: "USD" as const,
    // Onboarding specific fields
    annual_revenue: data.annualRevenue,
    primary_goals: data.primaryGoals || "",
    user_id: userId,
  };
};
