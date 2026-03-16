import { HugeiconsIcon } from "@hugeicons/react";
import { createFileRoute, Link, redirect } from "@tanstack/react-router";
import { AuthDivider } from "@/components/(auth)/auth-divider";
import { AuthLayout } from "@/components/(auth)/auth-layout";
import { OneTapInitializer } from "@/components/(auth)/one-tap-initializer";
import { PasskeyButton } from "@/components/(auth)/passkey-button";
import { SignInForm } from "@/components/(auth)/sign-in-form";
import { SocialProviders } from "@/components/(auth)/social-providers";
import ErrorComponent from "@/components/common/error-component";
import NotFound from "@/components/common/not-found";
import { authSearchSchema } from "@/lib/auth-search-schema";
import { BuildingIcon, MailIcon } from "@/lib/icons";

export const Route = createFileRoute("/(auth)/login")({
  validateSearch: authSearchSchema,
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

  return (
    <AuthLayout title="Sign in to Strait">
      {searchError ? (
        <div
          className="rounded-md bg-destructive/10 p-3 text-destructive text-sm"
          role="alert"
        >
          {searchError}
        </div>
      ) : null}

      <SignInForm
        onTwoFactorRequired={() => {
          window.location.href = `/two-factor${redirectTo ? `?redirect=${encodeURIComponent(redirectTo)}` : ""}`;
        }}
        redirectTo={redirectTo}
      />
      <AuthDivider />
      <SocialProviders redirectTo={redirectTo} />
      <AuthDivider label="or continue with" />
      <PasskeyButton />
      <Link
        className="inline-flex h-10 w-full items-center justify-center gap-2 rounded-md border border-input bg-background px-4 font-medium text-sm hover:bg-accent hover:text-accent-foreground"
        to="/magic-link"
      >
        <HugeiconsIcon className="size-4" icon={MailIcon} />
        Sign in with magic link
      </Link>
      <Link
        className="inline-flex h-10 w-full items-center justify-center gap-2 rounded-md border border-input bg-background px-4 font-medium text-sm hover:bg-accent hover:text-accent-foreground"
        to="/sso"
      >
        <HugeiconsIcon className="size-4" icon={BuildingIcon} />
        Sign in with SSO
      </Link>
      <p className="text-center text-muted-foreground text-sm">
        Don't have an account?{" "}
        <Link
          className="text-foreground underline-offset-4 hover:underline"
          to="/signup"
        >
          Sign up
        </Link>
      </p>

      <OneTapInitializer />
    </AuthLayout>
  );
}
