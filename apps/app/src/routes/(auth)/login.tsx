import { HugeiconsIcon } from "@hugeicons/react";
import { Alert, AlertDescription } from "@strait/ui/components/alert";
import { Button } from "@strait/ui/components/button";
import { createFileRoute, Link, redirect } from "@tanstack/react-router";
import { createServerFn } from "@tanstack/react-start";
import { useEffect } from "react";
import AuthDivider from "@/components/(auth)/auth-divider";
import AuthLayout from "@/components/(auth)/auth-layout";
import OneTapInitializer from "@/components/(auth)/one-tap-initializer";
import PasskeyButton from "@/components/(auth)/passkey-button";
import SignInForm from "@/components/(auth)/sign-in-form";
import SocialProviders from "@/components/(auth)/social-providers";
import ErrorComponent from "@/components/common/error-component";
import NotFound from "@/components/common/not-found";
import { authSearchSchema } from "@/lib/auth-search-schema";
import { MailIcon } from "@/lib/icons";
import { seo } from "@/lib/seo";
import { storeUtmParams } from "@/lib/utm";

const OAUTH_ERROR_MESSAGES: Record<string, string> = {
  OAuthSignin: "Could not start the sign-in process. Please try again.",
  OAuthCallback: "Something went wrong during sign-in. Please try again.",
  OAuthCreateAccount:
    "Could not create your account. An account with this email may already exist.",
  OAuthAccountNotLinked:
    "This email is already associated with another sign-in method. Try signing in with your original method.",
  AccessDenied: "Access was denied. You may have cancelled the sign-in.",
  Verification: "The verification link has expired or has already been used.",
};

function formatOAuthError(error: string): string {
  return OAUTH_ERROR_MESSAGES[error] ?? error;
}

const getSocialProviderAvailability = createServerFn({ method: "GET" }).handler(
  () => ({
    google: !!(
      process.env.GOOGLE_CLIENT_ID && process.env.GOOGLE_CLIENT_SECRET
    ),
    github: !!(
      process.env.GITHUB_CLIENT_ID && process.env.GITHUB_CLIENT_SECRET
    ),
  })
);

export const Route = createFileRoute("/(auth)/login")({
  validateSearch: authSearchSchema,
  beforeLoad: ({ context, search }) => {
    if (context.isAuthenticated) {
      throw redirect({ to: search.redirect ?? "/app" });
    }
  },
  loader: () => getSocialProviderAvailability(),
  head: () => ({ meta: seo({ title: "Sign in" }) }),
  errorComponent: ErrorComponent,
  notFoundComponent: NotFound,
  component: LoginPage,
});

function LoginPage() {
  const search = Route.useSearch();
  const providers = Route.useLoaderData();
  const { redirect: redirectTo, error: searchError } = search;

  useEffect(() => {
    storeUtmParams({
      utm_source: search.utm_source,
      utm_medium: search.utm_medium,
      utm_campaign: search.utm_campaign,
      utm_term: search.utm_term,
      utm_content: search.utm_content,
      ref: search.ref,
    });
  }, [search]);

  return (
    <AuthLayout title="Sign in to Strait">
      {searchError ? (
        <Alert variant="destructive">
          <AlertDescription>{formatOAuthError(searchError)}</AlertDescription>
        </Alert>
      ) : null}

      <SignInForm
        onTwoFactorRequired={() => {
          window.location.href = `/two-factor${redirectTo ? `?redirect=${encodeURIComponent(redirectTo)}` : ""}`;
        }}
        redirectTo={redirectTo}
      />
      {providers.google || providers.github ? (
        <>
          <AuthDivider />
          <SocialProviders providers={providers} redirectTo={redirectTo} />
        </>
      ) : null}
      <AuthDivider label="or continue with" />
      <PasskeyButton />
      <Button
        className="w-full"
        render={<Link to="/magic-link" />}
        variant="secondary-outline"
      >
        <HugeiconsIcon className="size-4" icon={MailIcon} />
        Sign in with magic link
      </Button>
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
