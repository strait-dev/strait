import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useRouter } from "@tanstack/react-router";
import { createServerFn } from "@tanstack/react-start";
import { getRequestHeaders } from "@tanstack/react-start/server";
import { auth } from "@/lib/auth.server";
import { createOrganizationServerFn } from "@/lib/organization-handler";
import { createProjectServerFn } from "@/lib/project-handler";
import { type OnboardingFormData, onboardingSchema } from "@/lib/schema";
import { authMiddleware } from "@/middlewares/auth";
import { transformOnboardingToOrgData } from "@/utils/onboarding";

type CompleteOnboardingResult = Awaited<
  ReturnType<typeof completeOnboardingServerFn>
>;

const completeOnboardingServerFn = createServerFn({ method: "POST" })
  .inputValidator((data: OnboardingFormData) => onboardingSchema.parse(data))
  .middleware([authMiddleware])
  .handler(async ({ context, data }) => {
    try {
      const orgData = transformOnboardingToOrgData(data, context.user.id);

      const organization = await createOrganizationServerFn({
        data: orgData,
      });

      if (!organization) {
        throw new Error("Failed to create organization during onboarding");
      }

      // Create a default project inside the new organization
      const project = await createProjectServerFn({
        data: {
          organizationId: organization.id,
          name: "Default",
          description: "Default project",
        },
      });

      const headers = getRequestHeaders();
      await auth.api.updateUser({
        body: {
          defaultOrganizationId: organization.id,
          activeProjectId: project?.id ?? "",
          onboarded: true,
        },
        headers,
      });

      return {
        organization,
        project,
        success: true,
      };
    } catch (error) {
      const message =
        error instanceof Error ? error.message : "Onboarding completion failed";
      throw new Error(`Failed to complete onboarding: ${message}`);
    }
  });

/** Skip onboarding without creating an organization. */
export const skipOnboardingServerFn = createServerFn({ method: "POST" })
  .middleware([authMiddleware])
  .handler(async () => {
    const headers = getRequestHeaders();
    await auth.api.updateUser({
      body: { onboarded: true },
      headers,
    });
    return { success: true };
  });

/**
 * Hook to complete onboarding (create organization + default project).
 * After success, redirects to /app.
 */
export const useCompleteOnboarding = () => {
  const queryClient = useQueryClient();
  const router = useRouter();

  return useMutation<CompleteOnboardingResult, Error, OnboardingFormData>({
    mutationKey: ["completeOnboarding"],
    mutationFn: (formData) =>
      completeOnboardingServerFn({
        data: formData,
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: ["organizations"],
      });

      router.navigate({ to: "/app" });
    },
    onError: (_error) => {
      // Error handled by toast in the component
    },
  });
};

/** Hook to skip onboarding without creating anything. */
export const useSkipOnboarding = () => {
  const router = useRouter();

  return useMutation({
    mutationKey: ["skipOnboarding"],
    mutationFn: () => skipOnboardingServerFn(),
    onSuccess: () => {
      router.navigate({ to: "/app" });
    },
  });
};
