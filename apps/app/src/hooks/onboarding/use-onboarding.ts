import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useRouter } from "@tanstack/react-router";
import { createServerFn } from "@tanstack/react-start";
import { getRequestHeaders } from "@tanstack/react-start/server";
import { auth } from "@/lib/auth.server";
import { createOrganizationServerFn } from "@/lib/organization-handler.server";
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

      const headers = getRequestHeaders();
      await auth.api.updateUser({
        body: {
          defaultOrganizationId: organization.id,
          onboarded: true,
        },
        headers,
      });

      return {
        organization,
        success: true,
      };
    } catch (error) {
      const message =
        error instanceof Error ? error.message : "Onboarding completion failed";
      throw new Error(`Failed to complete onboarding: ${message}`);
    }
  });

/**
 * Hook to complete onboarding (create organization only).
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

      // Redirect to app after onboarding
      router.navigate({ to: "/app" });
    },
    onError: (_error) => {
      // Error handled by toast in the component
    },
  });
};
