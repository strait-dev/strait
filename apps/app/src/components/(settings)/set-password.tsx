import { Button } from "@strait/ui/components/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@strait/ui/components/card";
import { Spinner } from "@strait/ui/components/spinner";
import { toast } from "@strait/ui/components/toast";
import { useEffect, useRef, useState } from "react";
import { authClient } from "@/lib/auth-client";
import { captureException } from "@/lib/sentry";

type Props = {
  email: string;
};

const RESEND_COOLDOWN_SECONDS = 30;

const SetPassword = ({ email }: Props) => {
  const [isLoading, setIsLoading] = useState(false);
  const [sent, setSent] = useState(false);
  const [cooldown, setCooldown] = useState(0);

  const intervalRef = useRef<ReturnType<typeof setInterval> | null>(null);

  useEffect(
    () => () => {
      if (intervalRef.current) {
        clearInterval(intervalRef.current);
      }
    },
    []
  );

  const startCooldown = () => {
    setCooldown(RESEND_COOLDOWN_SECONDS);
    intervalRef.current = setInterval(() => {
      setCooldown((prev) => {
        if (prev <= 1) {
          if (intervalRef.current) {
            clearInterval(intervalRef.current);
            intervalRef.current = null;
          }
          return 0;
        }
        return prev - 1;
      });
    }, 1000);
  };

  const handleRequestPasswordSetup = async () => {
    setIsLoading(true);
    try {
      const result = await authClient.requestPasswordReset({
        email,
        redirectTo: "/reset-password",
      });

      if (result.error) {
        toast.error(result.error.message ?? "Failed to send setup email.");
        setIsLoading(false);
        return;
      }

      setSent(true);
      startCooldown();
    } catch (error) {
      captureException(error);
      toast.error("Something went wrong. Please try again.");
    }
    setIsLoading(false);
  };

  if (sent) {
    return (
      <Card>
        <CardHeader>
          <CardTitle>Set a password</CardTitle>
          <CardDescription>
            We sent a password setup link to <strong>{email}</strong>. Check
            your email to set your password.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <Button
            disabled={isLoading || cooldown > 0}
            onClick={handleRequestPasswordSetup}
            variant="outline"
          >
            {isLoading ? <Spinner /> : null}
            {cooldown > 0
              ? `Resend in ${cooldown}s`
              : "Resend password setup email"}
          </Button>
        </CardContent>
      </Card>
    );
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>Set a password</CardTitle>
        <CardDescription>
          Your account was created with a social provider. Add a password so you
          can also sign in with email and password.
        </CardDescription>
      </CardHeader>
      <CardContent>
        <Button disabled={isLoading} onClick={handleRequestPasswordSetup}>
          {isLoading ? <Spinner /> : null}
          Send password setup email
        </Button>
      </CardContent>
    </Card>
  );
};

export default SetPassword;
