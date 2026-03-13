import { Button } from "@strait/ui/components/button.tsx";
import { createFileRoute, redirect } from "@tanstack/react-router";
import { useState } from "react";
import * as z from "zod";
import ErrorComponent from "@/components/common/error-component.tsx";
import NotFound from "@/components/common/not-found.tsx";
import { authClient } from "@/lib/auth-client.ts";

const loginSearchSchema = z.object({
  redirect: z.string().optional().catch(undefined),
  error: z.string().optional().catch(undefined),
});

export const Route = createFileRoute("/login")({
  validateSearch: loginSearchSchema,
  beforeLoad: ({ context, search }) => {
    if (context.isAuthenticated) {
      throw redirect({ to: search.redirect ?? "/app" });
    }
  },
  errorComponent: ErrorComponent,
  notFoundComponent: NotFound,
  component: LoginPage,
});

function LoginPage() {
  const { redirect: redirectTo, error: searchError } = Route.useSearch();
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(searchError ?? null);

  const handleGoogleSignIn = async () => {
    setIsLoading(true);
    setError(null);

    try {
      const result = await authClient.signIn.social({
        provider: "google",
        callbackURL: redirectTo ?? "/onboarding",
      });

      // Check if there was an error in the response
      if (result.error) {
        console.error("Sign in error:", result.error);
        setError(
          result.error.message || "Failed to sign in. Please try again."
        );
        setIsLoading(false);
      }
      // If successful, the browser will redirect to Google OAuth
      // so we don't need to do anything else here
    } catch (err) {
      console.error("Sign in error:", err);
      setError(
        err instanceof Error
          ? err.message
          : "Failed to sign in. Please try again."
      );
      setIsLoading(false);
    }
  };

  return (
    <div className="flex min-h-screen items-center justify-center bg-background">
      <div className="mx-auto w-full max-w-md space-y-6 p-8">
        <div className="space-y-2 text-center">
          <h1 className="font-bold text-3xl">Welcome to Strait</h1>
          <p className="text-muted-foreground">
            Sign in to continue to your AI writing assistant
          </p>
        </div>

        {error ? (
          <div className="rounded-md bg-destructive/10 p-3 text-destructive text-sm">
            {error}
          </div>
        ) : null}

        <Button
          className="w-full"
          disabled={isLoading}
          onClick={handleGoogleSignIn}
          size="lg"
        >
          {isLoading ? (
            <span className="flex items-center gap-2">
              <svg
                aria-hidden="true"
                className="h-5 w-5 animate-spin"
                fill="none"
                viewBox="0 0 24 24"
              >
                <circle
                  className="opacity-25"
                  cx="12"
                  cy="12"
                  r="10"
                  stroke="currentColor"
                  strokeWidth="4"
                />
                <path
                  className="opacity-75"
                  d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"
                  fill="currentColor"
                />
              </svg>
              Signing in...
            </span>
          ) : (
            <span className="flex items-center gap-2">
              <svg aria-hidden="true" className="h-5 w-5" viewBox="0 0 24 24">
                <path
                  d="M22.56 12.25c0-.78-.07-1.53-.2-2.25H12v4.26h5.92c-.26 1.37-1.04 2.53-2.21 3.31v2.77h3.57c2.08-1.92 3.28-4.74 3.28-8.09z"
                  fill="#4285F4"
                />
                <path
                  d="M12 23c2.97 0 5.46-.98 7.28-2.66l-3.57-2.77c-.98.66-2.23 1.06-3.71 1.06-2.86 0-5.29-1.93-6.16-4.53H2.18v2.84C3.99 20.53 7.7 23 12 23z"
                  fill="#34A853"
                />
                <path
                  d="M5.84 14.09c-.22-.66-.35-1.36-.35-2.09s.13-1.43.35-2.09V7.07H2.18C1.43 8.55 1 10.22 1 12s.43 3.45 1.18 4.93l2.85-2.22.81-.62z"
                  fill="#FBBC05"
                />
                <path
                  d="M12 5.38c1.62 0 3.06.56 4.21 1.64l3.15-3.15C17.45 2.09 14.97 1 12 1 7.7 1 3.99 3.47 2.18 7.07l3.66 2.84c.87-2.6 3.3-4.53 6.16-4.53z"
                  fill="#EA4335"
                />
              </svg>
              Continue with Google
            </span>
          )}
        </Button>

        <p className="text-center text-muted-foreground text-sm">
          By continuing, you agree to our Terms of Service and Privacy Policy
        </p>
      </div>
    </div>
  );
}
