import { Button } from "@strait/ui/components/button";
import { Checkbox } from "@strait/ui/components/checkbox";
import { Field, FieldLabel } from "@strait/ui/components/field";
import { Input } from "@strait/ui/components/input";
import {
  InputOTP,
  InputOTPGroup,
  InputOTPSlot,
} from "@strait/ui/components/input-otp";
import { Spinner } from "@strait/ui/components/spinner";
import { toast } from "@strait/ui/components/toast";
import { useState } from "react";
import { authClient } from "@/lib/auth-client";
import { captureSentryAuthError } from "@/lib/sentry";

type TwoFactorFormProps = {
  redirectTo?: string;
  disabled?: boolean;
};

type VerifyMode = "totp" | "backup";

const TwoFactorForm = ({ redirectTo, disabled }: TwoFactorFormProps) => {
  const [mode, setMode] = useState<VerifyMode>("totp");
  const [code, setCode] = useState("");
  const [backupCode, setBackupCode] = useState("");
  const [trustDevice, setTrustDevice] = useState(false);
  const [isSubmitting, setIsSubmitting] = useState(false);

  const handleVerifyTotp = async () => {
    if (code.length !== 6) {
      return;
    }
    setIsSubmitting(true);

    try {
      const result = await authClient.twoFactor.verifyTotp({
        code,
        trustDevice,
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

  const handleVerifyBackup = async () => {
    if (!backupCode.trim()) {
      return;
    }
    setIsSubmitting(true);

    try {
      const result = await authClient.twoFactor.verifyBackupCode({
        code: backupCode.trim(),
      });

      if (result.error) {
        captureSentryAuthError(result.error, {
          operation: "2fa-verify",
          provider: "email",
        });
        toast.error(
          result.error.message ?? "Invalid backup code. Please try again."
        );
        setBackupCode("");
        setIsSubmitting(false);
        return;
      }

      window.location.href = redirectTo ?? "/app";
    } catch (error) {
      captureSentryAuthError(error, {
        operation: "2fa-verify",
        provider: "email",
      });
      toast.error("Verification failed. Please try again.");
      setBackupCode("");
      setIsSubmitting(false);
    }
  };

  return (
    <div className="flex flex-col items-center gap-6">
      {mode === "totp" && (
        <>
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

          <Field className="w-full justify-center" orientation="horizontal">
            <Checkbox
              checked={trustDevice}
              id="trust-device"
              onCheckedChange={(checked) => setTrustDevice(checked === true)}
            />
            <FieldLabel
              className="text-muted-foreground"
              htmlFor="trust-device"
            >
              Trust this device for 30 days
            </FieldLabel>
          </Field>

          <Button
            className="w-full"
            disabled={disabled || isSubmitting || code.length !== 6}
            onClick={handleVerifyTotp}
            variant="brand-solid"
          >
            {isSubmitting ? <Spinner /> : null}
            Verify
          </Button>

          <Button
            onClick={() => {
              setMode("backup");
              setCode("");
            }}
            type="button"
            variant="link"
          >
            Use a backup code instead
          </Button>
        </>
      )}

      {mode === "backup" && (
        <>
          <p className="text-center text-muted-foreground text-sm">
            Enter one of your backup codes.
          </p>

          <Input
            className="text-center font-mono"
            disabled={disabled || isSubmitting}
            onChange={(e) => setBackupCode(e.target.value)}
            placeholder="Enter backup code"
            value={backupCode}
          />

          <Button
            className="w-full"
            disabled={disabled || isSubmitting || !backupCode.trim()}
            onClick={handleVerifyBackup}
            variant="brand-solid"
          >
            {isSubmitting ? <Spinner /> : null}
            Verify backup code
          </Button>

          <Button
            onClick={() => {
              setMode("totp");
              setBackupCode("");
            }}
            type="button"
            variant="link"
          >
            Use authenticator code instead
          </Button>
        </>
      )}
    </div>
  );
};

export default TwoFactorForm;
