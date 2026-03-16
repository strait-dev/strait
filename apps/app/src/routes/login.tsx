import { Button } from "@strait/ui/components/button";
import { createFileRoute, redirect, useNavigate } from "@tanstack/react-router";
import { useCallback } from "react";
import * as z from "zod";
import { AuthDivider } from "@/components/(auth)/auth-divider";
import { AuthLayout } from "@/components/(auth)/auth-layout";
import { ForgotPasswordForm } from "@/components/(auth)/forgot-password-form";
import { MagicLinkForm } from "@/components/(auth)/magic-link-form";
import { OneTapInitializer } from "@/components/(auth)/one-tap-initializer";
import { PasskeyButton } from "@/components/(auth)/passkey-button";
import { SignInForm } from "@/components/(auth)/sign-in-form";
import { SignUpForm } from "@/components/(auth)/sign-up-form";
import { SocialProviders } from "@/components/(auth)/social-providers";
import { SsoForm } from "@/components/(auth)/sso-form";
import { TwoFactorForm } from "@/components/(auth)/two-factor-form";
import ErrorComponent from "@/components/common/error-component";
import NotFound from "@/components/common/not-found";

export type AuthView =
  | "signin"
  | "signup"
  | "magic-link"
  | "forgot-password"
  | "sso"
  | "2fa";

const loginSearchSchema = z.object({
  redirect: z.string().optional().catch(undefined),
  error: z.string().optional().catch(undefined),
  view: z
    .enum(["signin", "signup", "magic-link", "forgot-password", "sso", "2fa"])
    .optional()
    .catch("signin"),
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

const VIEW_CONFIG: Record<AuthView, { title: string; description?: string }> = {
  signin: { title: "Sign in to Strait" },
  signup: { title: "Create your account" },
  "magic-link": { title: "Magic link sign in" },
  "forgot-password": { title: "Reset your password" },
  sso: { title: "Enterprise SSO" },
  "2fa": { title: "Two-factor verification" },
};

function LoginPage() {
  const {
    redirect: redirectTo,
    error: searchError,
    view = "signin",
  } = Route.useSearch();
  const navigate = useNavigate();

  const onNavigate = useCallback(
    (newView: AuthView) => {
      navigate({
        to: "/login",
        search: (prev) => ({ ...prev, view: newView }),
        replace: true,
      });
    },
    [navigate]
  );

  const { title, description } = VIEW_CONFIG[view];

  return (
    <AuthLayout description={description} title={title}>
      {searchError ? (
        <div
          className="rounded-md bg-destructive/10 p-3 text-destructive text-sm"
          role="alert"
        >
          {searchError}
        </div>
      ) : null}

      {view === "signin" && (
        <>
          <SignInForm
            onNavigate={onNavigate}
            onTwoFactorRequired={() => onNavigate("2fa")}
            redirectTo={redirectTo}
          />
          <AuthDivider />
          <SocialProviders redirectTo={redirectTo} />
          <AuthDivider label="or continue with" />
          <PasskeyButton redirectTo={redirectTo} />
          <div className="flex gap-2">
            <Button
              className="flex-1"
              onClick={() => onNavigate("magic-link")}
              size="sm"
              variant="ghost"
            >
              Magic link
            </Button>
            <Button
              className="flex-1"
              onClick={() => onNavigate("sso")}
              size="sm"
              variant="ghost"
            >
              Enterprise SSO
            </Button>
          </div>
          <p className="text-center text-muted-foreground text-sm">
            Don't have an account?{" "}
            <button
              className="text-foreground underline-offset-4 hover:underline"
              onClick={() => onNavigate("signup")}
              type="button"
            >
              Sign up
            </button>
          </p>
        </>
      )}

      {view === "signup" && (
        <>
          <SignUpForm redirectTo={redirectTo} />
          <p className="text-center text-muted-foreground text-sm">
            Already have an account?{" "}
            <button
              className="text-foreground underline-offset-4 hover:underline"
              onClick={() => onNavigate("signin")}
              type="button"
            >
              Sign in
            </button>
          </p>
        </>
      )}

      {view === "magic-link" && (
        <>
          <MagicLinkForm redirectTo={redirectTo} />
          <p className="text-center text-muted-foreground text-sm">
            <button
              className="text-foreground underline-offset-4 hover:underline"
              onClick={() => onNavigate("signin")}
              type="button"
            >
              Back to sign in
            </button>
          </p>
        </>
      )}

      {view === "forgot-password" && (
        <>
          <ForgotPasswordForm />
          <p className="text-center text-muted-foreground text-sm">
            <button
              className="text-foreground underline-offset-4 hover:underline"
              onClick={() => onNavigate("signin")}
              type="button"
            >
              Back to sign in
            </button>
          </p>
        </>
      )}

      {view === "sso" && (
        <>
          <SsoForm redirectTo={redirectTo} />
          <p className="text-center text-muted-foreground text-sm">
            <button
              className="text-foreground underline-offset-4 hover:underline"
              onClick={() => onNavigate("signin")}
              type="button"
            >
              Back to sign in
            </button>
          </p>
        </>
      )}

      {view === "2fa" && (
        <>
          <TwoFactorForm redirectTo={redirectTo} />
          <p className="text-center text-muted-foreground text-sm">
            <button
              className="text-foreground underline-offset-4 hover:underline"
              onClick={() => onNavigate("signin")}
              type="button"
            >
              Back to sign in
            </button>
          </p>
        </>
      )}

      <OneTapInitializer />
    </AuthLayout>
  );
}
