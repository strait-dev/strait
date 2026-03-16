import { createFileRoute, Link, redirect } from "@tanstack/react-router";
import { AuthLayout } from "@/components/(auth)/auth-layout";
import { SignUpForm } from "@/components/(auth)/sign-up-form";
import ErrorComponent from "@/components/common/error-component";
import NotFound from "@/components/common/not-found";
import { authSearchSchema } from "@/lib/auth-search-schema";

export const Route = createFileRoute("/(auth)/signup")({
  validateSearch: authSearchSchema,
  beforeLoad: ({ context, search }) => {
    if (context.isAuthenticated) {
      throw redirect({ to: search.redirect ?? "/app" });
    }
  },
  errorComponent: ErrorComponent,
  notFoundComponent: NotFound,
  component: SignUpPage,
});

function SignUpPage() {
  const { redirect: redirectTo } = Route.useSearch();

  return (
    <AuthLayout title="Create your account">
      <SignUpForm redirectTo={redirectTo} />
      <p className="text-center text-muted-foreground text-sm">
        Already have an account?{" "}
        <Link
          className="text-foreground underline-offset-4 hover:underline"
          to="/login"
        >
          Sign in
        </Link>
      </p>
      <p className="text-pretty text-center text-muted-foreground text-xs">
        By creating an account, you agree to our{" "}
        <a
          className="text-foreground underline-offset-4 hover:underline"
          href="/terms"
          rel="noopener noreferrer"
          target="_blank"
        >
          Terms of Service
        </a>{" "}
        and{" "}
        <a
          className="text-foreground underline-offset-4 hover:underline"
          href="/privacy"
          rel="noopener noreferrer"
          target="_blank"
        >
          Privacy Policy
        </a>
        .
      </p>
    </AuthLayout>
  );
}
