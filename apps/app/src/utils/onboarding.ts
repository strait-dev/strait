import { nanoid } from "nanoid";
import type { OnboardingFormData } from "../lib/schema";
import { MAX_SLUG_LENGTH, ORGANIZATION_SLUG_LENGTH } from "./constants";

/**
 * Regex for removing trailing hyphens from slugs
 */
const TRAILING_HYPHENS_REGEX = /-+$/;

/**
 * Generates a URL-friendly slug from a workspace name
 */
const generateSlug = (name: string): string => {
  return name
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "")
    .slice(0, MAX_SLUG_LENGTH)
    .replace(TRAILING_HYPHENS_REGEX, "");
};

/**
 * Transforms onboarding form data into organization creation payload.
 * Stores orchestration-specific context (use cases, team size, environment)
 * as structured metadata.
 */
export const transformOnboardingToOrgData = (
  data: OnboardingFormData,
  userId: string
) => {
  const workspaceName = data.workspaceName || "My Workspace";
  const baseSlug = generateSlug(workspaceName);
  const slug = baseSlug
    ? `${baseSlug}-${nanoid(ORGANIZATION_SLUG_LENGTH)}`
    : `ws-${nanoid(ORGANIZATION_SLUG_LENGTH)}`;

  return {
    id: nanoid(),
    name: workspaceName,
    slug,
    logo: "",
    metadata: JSON.stringify({
      useCases: data.useCases,
      teamSize: data.teamSize,
      environment: data.environment,
    }),
    description: data.primaryGoals || "",
    user_id: userId,
  };
};
