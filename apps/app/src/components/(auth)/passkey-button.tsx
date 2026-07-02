import { HugeiconsIcon } from "@hugeicons/react";
import { Button } from "@strait/ui/components/button";
import { Spinner } from "@strait/ui/components/spinner";
import { toast } from "@strait/ui/components/toast";
import { useState } from "react";
import { getPostHog } from "@/lib/analytics";
import { authClient } from "@/lib/auth-client";
import { KeyIcon } from "@/lib/icons";
import { captureSentryAuthError } from "@/lib/sentry";

type PasskeyButtonProps = {
  redirectTo?: string;
  disabled?: boolean;
};

const PasskeyButton = ({ disabled }: PasskeyButtonProps) => {
  const [isLoading, setIsLoading] = useState(false);

  const handlePasskeySignIn = async () => {
    setIsLoading(true);
    try {
      const result = await authClient.signIn.passkey();

      if (!result?.error) {
        getPostHog()?.capture("auth_signed_in", { method: "passkey" });
      }

      if (result?.error) {
        captureSentryAuthError(result.error, {
          operation: "passkey",
          provider: "passkey",
        });
        toast.error(
          String(
            result.error.message ?? "Passkey sign in failed. Please try again."
          )
        );
      }
    } catch (error) {
      captureSentryAuthError(error, {
        operation: "passkey",
        provider: "passkey",
      });
      toast.error("Passkey sign in failed. Please try again.");
    }
    setIsLoading(false);
  };

  return (
    <Button
      className="w-full"
      disabled={disabled || isLoading}
      onClick={handlePasskeySignIn}
      variant="secondary-outline"
    >
      {isLoading ? (
        <Spinner />
      ) : (
        <HugeiconsIcon className="size-4" icon={KeyIcon} />
      )}
      Sign in with passkey
    </Button>
  );
};

export default PasskeyButton;
