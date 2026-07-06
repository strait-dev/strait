import { HugeiconsIcon } from "@hugeicons/react";
import { Button } from "@strait/ui/components/button";
import { EmptyMedia } from "@strait/ui/components/empty";
import { Spinner } from "@strait/ui/components/spinner";
import { createFileRoute, Link, redirect } from "@tanstack/react-router";
import { useEffect, useState } from "react";
import * as z from "zod";
import AuthLayout from "@/components/(auth)/auth-layout";
import ErrorComponent from "@/components/common/error-component";
import NotFound from "@/components/common/not-found";
import { authClient } from "@/lib/auth-client";
import { CheckCircleIcon, XCircleIcon } from "@/lib/icons";
import { seo } from "@/lib/seo";

const verifyEmailSearchSchema = z.object({
  token: z.string(),
});

export const Route = createFileRoute("/(auth)/verify-email")({
  validateSearch: verifyEmailSearchSchema,
  beforeLoad: ({ context }) => {
    if (context.isAuthenticated) {
      throw redirect({ to: "/app" });
    }
  },
  head: () => ({ meta: seo({ title: "Verify email" }) }),
  errorComponent: ErrorComponent,
  notFoundComponent: NotFound,
  component: VerifyEmailPage,
});

function VerifyEmailPage() {
  const { token } = Route.useSearch();
  const [status, setStatus] = useState<"verifying" | "success" | "error">(
    "verifying"
  );
  const [errorMessage, setErrorMessage] = useState<string | null>(null);

  useEffect(() => {
    const verify = async () => {
      const result = await authClient.verifyEmail({ query: { token } });

      if (result.error) {
        setErrorMessage(
          result.error.message ??
            "Verification failed. The link may be invalid or expired."
        );
        setStatus("error");
        return;
      }

      setStatus("success");
    };

    verify();
  }, [token]);

  return (
    <AuthLayout title="Email verification">
      {status === "verifying" && (
        <div className="flex flex-col items-center gap-3 py-4 text-center">
          <Spinner className="text-primary" size="lg" />
          <p className="text-muted-foreground text-sm">
            Verifying your email...
          </p>
        </div>
      )}

      {status === "success" && (
        <div className="flex flex-col items-center gap-4 py-4 text-center">
          <EmptyMedia media="icon" size="lg" variant="success">
            <HugeiconsIcon className="size-6" icon={CheckCircleIcon} />
          </EmptyMedia>
          <p className="font-medium text-foreground text-sm">
            Email verified successfully
          </p>
          <p className="text-muted-foreground text-sm">
            Your email has been verified. You can now sign in.
          </p>
          <Button render={<Link to="/login" />} variant="brand-solid">
            Sign in
          </Button>
        </div>
      )}

      {status === "error" && (
        <div className="flex flex-col items-center gap-4 py-4 text-center">
          <EmptyMedia media="icon" size="lg" variant="destructive">
            <HugeiconsIcon className="size-6" icon={XCircleIcon} />
          </EmptyMedia>
          <p className="font-medium text-foreground text-sm">
            Verification failed
          </p>
          <p className="text-muted-foreground text-sm">{errorMessage}</p>
          <Button render={<Link to="/login" />} variant="brand-solid">
            Back to sign in
          </Button>
        </div>
      )}
    </AuthLayout>
  );
}
