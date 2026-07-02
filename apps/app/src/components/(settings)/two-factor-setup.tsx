import { Button } from "@strait/ui/components/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@strait/ui/components/card";
import { CodeBlock } from "@strait/ui/components/code-block";
import { Field, FieldError, FieldLabel } from "@strait/ui/components/field";
import { Frame, FramePanel } from "@strait/ui/components/frame";
import {
  InputOTP,
  InputOTPGroup,
  InputOTPSlot,
} from "@strait/ui/components/input-otp";
import { PasswordInput } from "@strait/ui/components/password-input";
import { Spinner } from "@strait/ui/components/spinner";
import { toast } from "@strait/ui/components/toast";
import { useQueryClient } from "@tanstack/react-query";
import QRCode from "qrcode";
import { useCallback, useState } from "react";
import { queryKeys } from "@/hooks/query-keys";
import { authClient } from "@/lib/auth-client";
import { captureException } from "@/lib/sentry";

type SetupStep = "idle" | "qr" | "verify" | "backup-codes" | "disable";

type Props = {
  enabled: boolean;
};

const TwoFactorSetup = ({ enabled }: Props) => {
  const queryClient = useQueryClient();
  const [step, setStep] = useState<SetupStep>("idle");
  const [password, setPassword] = useState("");
  const [totpQrDataUrl, setTotpQrDataUrl] = useState("");
  const [backupCodes, setBackupCodes] = useState<string[]>([]);
  const [verifyCode, setVerifyCode] = useState("");
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const handleEnable = useCallback(async () => {
    if (!password) {
      setError("Password is required");
      return;
    }
    setIsLoading(true);
    setError(null);

    try {
      const result = await authClient.twoFactor.enable({
        password,
      });

      if (result.error) {
        setError(result.error.message ?? "Failed to enable 2FA.");
        setIsLoading(false);
        return;
      }

      const nextTotpUri = result.data?.totpURI ?? "";
      setTotpQrDataUrl(
        nextTotpUri
          ? await QRCode.toDataURL(nextTotpUri, {
              errorCorrectionLevel: "M",
              margin: 2,
              width: 200,
            })
          : ""
      );
      setBackupCodes(result.data?.backupCodes ?? []);
      setStep("qr");
    } catch (err) {
      captureException(err);
      setError("Something went wrong.");
    }
    setIsLoading(false);
  }, [password]);

  const handleVerify = useCallback(async () => {
    if (verifyCode.length !== 6) {
      return;
    }
    setIsLoading(true);

    try {
      const result = await authClient.twoFactor.verifyTotp({
        code: verifyCode,
      });

      if (result.error) {
        toast.error(result.error.message ?? "Invalid code.");
        setVerifyCode("");
        setIsLoading(false);
        return;
      }

      setStep("backup-codes");
      queryClient.invalidateQueries({ queryKey: queryKeys.auth._def });
    } catch (err) {
      captureException(err);
      toast.error("Verification failed.");
    }
    setIsLoading(false);
  }, [verifyCode, queryClient]);

  const handleDisable = useCallback(async () => {
    if (!password) {
      setError("Password is required");
      return;
    }
    setIsLoading(true);
    setError(null);

    try {
      const result = await authClient.twoFactor.disable({
        password,
      });

      if (result.error) {
        setError(result.error.message ?? "Failed to disable 2FA.");
        setIsLoading(false);
        return;
      }

      toast.success("Two-factor authentication disabled.");
      setStep("idle");
      setPassword("");
      setTotpQrDataUrl("");
      queryClient.invalidateQueries({ queryKey: queryKeys.auth._def });
    } catch (err) {
      captureException(err);
      setError("Something went wrong.");
    }
    setIsLoading(false);
  }, [password, queryClient]);

  if (step === "qr") {
    return (
      <Card>
        <CardHeader>
          <CardTitle>Set up authenticator</CardTitle>
          <CardDescription>
            Scan this QR code with your authenticator app (Google Authenticator,
            Authy, etc.), then enter the 6-digit code to verify.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div className="flex flex-col items-center gap-6">
            {totpQrDataUrl && (
              <Frame>
                <FramePanel>
                  <img
                    alt="2FA QR Code"
                    height={200}
                    src={totpQrDataUrl}
                    width={200}
                  />
                </FramePanel>
              </Frame>
            )}

            <div className="w-full text-center">
              <p className="mb-3 text-muted-foreground text-sm">
                Enter the code from your authenticator app
              </p>
              <div className="flex justify-center">
                <InputOTP
                  disabled={isLoading}
                  maxLength={6}
                  onChange={setVerifyCode}
                  value={verifyCode}
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
              </div>
            </div>

            <Button
              className="w-full"
              disabled={isLoading || verifyCode.length !== 6}
              onClick={handleVerify}
            >
              {isLoading ? <Spinner /> : null}
              Verify and enable
            </Button>
          </div>
        </CardContent>
      </Card>
    );
  }

  if (step === "backup-codes") {
    return (
      <Card>
        <CardHeader>
          <CardTitle>Save your backup codes</CardTitle>
          <CardDescription>
            Store these codes in a safe place. Each code can only be used once
            to sign in if you lose access to your authenticator app.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div className="flex flex-col gap-4">
            <CodeBlock code={backupCodes.join("\n")} />
            <Button
              onClick={() => {
                setStep("idle");
                setPassword("");
                setVerifyCode("");
              }}
            >
              Done
            </Button>
          </div>
        </CardContent>
      </Card>
    );
  }

  if (step === "disable") {
    return (
      <Card>
        <CardHeader>
          <CardTitle>Disable two-factor authentication</CardTitle>
          <CardDescription>
            Enter your password to confirm disabling 2FA.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <div className="flex flex-col gap-4">
            <Field className="w-full">
              <FieldLabel htmlFor="disable-2fa-password">Password</FieldLabel>
              <PasswordInput
                autoComplete="current-password"
                id="disable-2fa-password"
                onChange={(e) => setPassword(e.target.value)}
                placeholder="Enter your password"
                value={password}
              />
              {error && <FieldError>{error}</FieldError>}
            </Field>
            <div className="flex gap-2">
              <Button
                className="flex-1"
                onClick={() => {
                  setStep("idle");
                  setPassword("");
                  setError(null);
                }}
                variant="outline"
              >
                Cancel
              </Button>
              <Button
                className="flex-1"
                disabled={isLoading || !password}
                onClick={handleDisable}
                variant="destructive"
              >
                {isLoading ? <Spinner /> : null}
                Disable 2FA
              </Button>
            </div>
          </div>
        </CardContent>
      </Card>
    );
  }

  // idle state
  return (
    <Card>
      <CardHeader>
        <CardTitle>Two-factor authentication</CardTitle>
        <CardDescription>
          {enabled
            ? "Two-factor authentication is enabled on your account."
            : "Add an extra layer of security by enabling two-factor authentication."}
        </CardDescription>
      </CardHeader>
      <CardContent>
        {enabled ? (
          <Button
            onClick={() => {
              setStep("disable");
              setPassword("");
              setError(null);
            }}
            variant="destructive"
          >
            Disable 2FA
          </Button>
        ) : (
          <div className="flex flex-col gap-4">
            <Field className="w-full">
              <FieldLabel htmlFor="enable-2fa-password">Password</FieldLabel>
              <PasswordInput
                autoComplete="current-password"
                id="enable-2fa-password"
                onChange={(e) => setPassword(e.target.value)}
                placeholder="Enter your password to enable 2FA"
                value={password}
              />
              {error && <FieldError>{error}</FieldError>}
            </Field>
            <Button
              className="w-fit"
              disabled={isLoading || !password}
              onClick={handleEnable}
            >
              {isLoading ? <Spinner /> : null}
              Enable 2FA
            </Button>
          </div>
        )}
      </CardContent>
    </Card>
  );
};

export default TwoFactorSetup;
