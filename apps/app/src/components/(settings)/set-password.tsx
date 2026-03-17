import { HugeiconsIcon } from "@hugeicons/react";
import { Button } from "@strait/ui/components/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@strait/ui/components/card";
import { toast } from "@strait/ui/components/toast/index";
import { useState } from "react";
import { authClient } from "@/lib/auth-client";
import { LoadingIcon } from "@/lib/icons";
import { captureException } from "@/lib/sentry";

type Props = {
  email: string;
};

const SetPassword = ({ email }: Props) => {
  const [isLoading, setIsLoading] = useState(false);
  const [sent, setSent] = useState(false);

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
    } catch (error) {
      captureException(error);
      toast.error("Something went wrong. Please try again.");
    } finally {
      setIsLoading(false);
    }
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
          {isLoading ? (
            <HugeiconsIcon className="size-4 animate-spin" icon={LoadingIcon} />
          ) : null}
          Send password setup email
        </Button>
      </CardContent>
    </Card>
  );
};

export default SetPassword;
