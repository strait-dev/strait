import { createFileRoute, Link, redirect } from "@tanstack/react-router";
import AuthLayout from "@/components/(auth)/auth-layout";
import ForgotPasswordForm from "@/components/(auth)/forgot-password-form";
import ErrorComponent from "@/components/common/error-component";
import NotFound from "@/components/common/not-found";
import { authSearchSchema } from "@/lib/auth-search-schema";
import { seoHead } from "@/lib/seo";

export const Route = createFileRoute("/(auth)/forgot-password")({
  validateSearch: authSearchSchema,
  beforeLoad: ({ context, search }) => {
    if (context.isAuthenticated) {
      throw redirect({ to: search.redirect ?? "/app" });
    }
  },
  head: () =>
    seoHead({
      title: "Forgot password",
      description: "Reset the password for your Strait account.",
      path: "/forgot-password",
    }),
  errorComponent: ErrorComponent,
  notFoundComponent: NotFound,
  component: ForgotPasswordPage,
});

function ForgotPasswordPage() {
  return (
    <AuthLayout title="Reset your password">
      <ForgotPasswordForm />
      <p className="text-center text-muted-foreground text-sm">
        <Link
          className="text-foreground underline-offset-4 hover:underline"
          to="/login"
        >
          Back to sign in
        </Link>
      </p>
    </AuthLayout>
  );
}
