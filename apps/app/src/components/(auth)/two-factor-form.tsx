import { HugeiconsIcon } from "@hugeicons/react";
import { Button } from "@strait/ui/components/button";
import {
  InputOTP,
  InputOTPGroup,
  InputOTPSlot,
} from "@strait/ui/components/input-otp";
import { toast } from "@strait/ui/components/toast/index";
import { useState } from "react";
import { authClient } from "@/lib/auth-client";
import { LoadingIcon } from "@/lib/icons";
import { captureSentryAuthError } from "@/lib/sentry";

type TwoFactorFormProps = {
  redirectTo?: string;
  disabled?: boolean;
};

export const TwoFactorForm = ({ redirectTo, disabled }: TwoFactorFormProps) => {
  const [code, setCode] = useState("");
  const [isSubmitting, setIsSubmitting] = useState(false);

  const handleVerify = async () => {
    if (code.length !== 6) {
      return;
    }
    setIsSubmitting(true);

    try {
      const result = await authClient.twoFactor.verifyTotp({
        code,
      });

      if (result.error) {
        captureSentryAuthError(result.error, {
          operation: "2fa-verify",
          provider: "email",
        });
        toast.error(result.error.message ?? "Invalid code. Please try again.");
        setCode("");
        setIsSubmitting(false);
        return;
      }

      // Redirect on success
      window.location.href = redirectTo ?? "/app";
    } catch (error) {
      captureSentryAuthError(error, {
        operation: "2fa-verify",
        provider: "email",
      });
      toast.error("Verification failed. Please try again.");
      setCode("");
      setIsSubmitting(false);
    }
  };

  return (
    <div className="flex flex-col items-center gap-6">
      <p className="text-center text-muted-foreground text-sm">
        Enter the 6-digit code from your authenticator app.
      </p>

      <InputOTP
        disabled={disabled || isSubmitting}
        maxLength={6}
        onChange={setCode}
        value={code}
      >
        <InputOTPGroup>
          <InputOTPSlot index={0} />
          <InputOTPSlot index={1} />
          <InputOTPSlot index={2} />
          <InputOTPSlot index={3} />
          <InputOTPSlot index={4} />
          <InputOTPSlot index={5} />
        </InputOTPGroup>
      </InputOTP>

      <Button
        className="w-full"
        disabled={disabled || isSubmitting || code.length !== 6}
        onClick={handleVerify}
        size="lg"
      >
        {isSubmitting ? (
          <HugeiconsIcon className="size-4 animate-spin" icon={LoadingIcon} />
        ) : null}
        Verify
      </Button>
    </div>
  );
};
