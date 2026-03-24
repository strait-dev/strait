import { HugeiconsIcon } from "@hugeicons/react";
import { Button } from "@strait/ui/components/button";
import { toast } from "@strait/ui/components/toast/index";
import { useState } from "react";
import { authClient } from "@/lib/auth-client";
import { KeyIcon, LoadingIcon } from "@/lib/icons";
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
    } finally {
      setIsLoading(false);
    }
  };

  return (
    <Button
      className="w-full"
      disabled={disabled || isLoading}
      onClick={handlePasskeySignIn}
      variant="outline"
    >
      {isLoading ? (
        <HugeiconsIcon className="size-4 animate-spin" icon={LoadingIcon} />
      ) : (
        <HugeiconsIcon className="size-4" icon={KeyIcon} />
      )}
      Sign in with passkey
    </Button>
  );
};

export default PasskeyButton;
