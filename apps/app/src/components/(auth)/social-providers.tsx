import { HugeiconsIcon } from "@hugeicons/react";
import { Button } from "@strait/ui/components/button";
import { Spinner } from "@strait/ui/components/spinner";
import { toast } from "@strait/ui/components/toast";
import { useState } from "react";
import { getPostHog } from "@/lib/analytics";
import { authClient } from "@/lib/auth-client";
import { GlobeIcon, WorkflowIcon } from "@/lib/icons";
import { captureSentryAuthError } from "@/lib/sentry";

type SocialProvidersProps = {
  redirectTo?: string;
  disabled?: boolean;
  providers: {
    google: boolean;
    github: boolean;
  };
};

const SocialProviders = ({
  redirectTo,
  disabled,
  providers,
}: SocialProvidersProps) => {
  const [loadingProvider, setLoadingProvider] = useState<
    "google" | "github" | null
  >(null);

  const handleSocialSignIn = async (provider: "google" | "github") => {
    setLoadingProvider(provider);
    getPostHog()?.capture("auth_signed_in", { method: provider });
    try {
      const result = await authClient.signIn.social({
        provider,
        callbackURL: redirectTo ?? "/app",
      });
      if (result.error) {
        captureSentryAuthError(result.error, {
          operation: "signin",
          provider,
        });
        toast.error(
          result.error.message ?? "Failed to sign in. Please try again."
        );
        setLoadingProvider(null);
      }
    } catch (error) {
      captureSentryAuthError(error, { operation: "signin", provider });
      toast.error("Failed to sign in. Please try again.");
      setLoadingProvider(null);
    }
  };

  const isLoading = loadingProvider !== null;

  if (!(providers.google || providers.github)) {
    return null;
  }

  return (
    <div className="flex flex-col gap-3">
      {providers.google ? (
        <Button
          className="w-full"
          disabled={disabled || isLoading}
          onClick={() => handleSocialSignIn("google")}
          variant="secondary-outline"
        >
          {loadingProvider === "google" ? (
            <Spinner />
          ) : (
            <span data-icon="inline-start">
              <HugeiconsIcon aria-hidden="true" icon={GlobeIcon} size={18} />
            </span>
          )}
          Continue with Google
        </Button>
      ) : null}

      {providers.github ? (
        <Button
          className="w-full"
          disabled={disabled || isLoading}
          onClick={() => handleSocialSignIn("github")}
          variant="secondary-outline"
        >
          {loadingProvider === "github" ? (
            <Spinner />
          ) : (
            <span data-icon="inline-start">
              <HugeiconsIcon aria-hidden="true" icon={WorkflowIcon} size={18} />
            </span>
          )}
          Continue with GitHub
        </Button>
      ) : null}
    </div>
  );
};

export default SocialProviders;
