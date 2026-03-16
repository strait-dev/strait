import { createFileRoute, Link, redirect } from "@tanstack/react-router";
import { useEffect, useState } from "react";
import * as z from "zod";
import { AuthLayout } from "@/components/(auth)/auth-layout";
import ErrorComponent from "@/components/common/error-component";
import NotFound from "@/components/common/not-found";
import { authClient } from "@/lib/auth-client";

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
          <div className="size-6 animate-spin rounded-full border-2 border-muted-foreground border-t-primary" />
          <p className="text-muted-foreground text-sm">
            Verifying your email...
          </p>
        </div>
      )}

      {status === "success" && (
        <div className="flex flex-col items-center gap-4 py-4 text-center">
          <div className="rounded-full bg-primary/10 p-3">
            <svg
              aria-hidden="true"
              className="size-6 text-primary"
              fill="none"
              stroke="currentColor"
              strokeWidth="2"
              viewBox="0 0 24 24"
            >
              <path
                d="M5 13l4 4L19 7"
                strokeLinecap="round"
                strokeLinejoin="round"
              />
            </svg>
          </div>
          <p className="font-medium text-foreground text-sm">
            Email verified successfully
          </p>
          <p className="text-muted-foreground text-sm">
            Your email has been verified. You can now sign in.
          </p>
          <Link
            className="inline-flex h-10 items-center justify-center rounded-md bg-primary px-4 font-medium text-primary-foreground text-sm hover:bg-primary/90"
            to="/login"
          >
            Sign in
          </Link>
        </div>
      )}

      {status === "error" && (
        <div className="flex flex-col items-center gap-4 py-4 text-center">
          <div className="rounded-full bg-destructive/10 p-3">
            <svg
              aria-hidden="true"
              className="size-6 text-destructive"
              fill="none"
              stroke="currentColor"
              strokeWidth="2"
              viewBox="0 0 24 24"
            >
              <path
                d="M6 18L18 6M6 6l12 12"
                strokeLinecap="round"
                strokeLinejoin="round"
              />
            </svg>
          </div>
          <p className="font-medium text-foreground text-sm">
            Verification failed
          </p>
          <p className="text-muted-foreground text-sm">{errorMessage}</p>
          <Link
            className="inline-flex h-10 items-center justify-center rounded-md bg-primary px-4 font-medium text-primary-foreground text-sm hover:bg-primary/90"
            to="/login"
          >
            Back to sign in
          </Link>
        </div>
      )}
    </AuthLayout>
  );
}
