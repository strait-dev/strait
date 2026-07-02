import { createFileRoute, Link, redirect } from "@tanstack/react-router";
import AuthLayout from "@/components/(auth)/auth-layout";
import TwoFactorForm from "@/components/(auth)/two-factor-form";
import ErrorComponent from "@/components/common/error-component";
import NotFound from "@/components/common/not-found";
import { authSearchSchema } from "@/lib/auth-search-schema";

export const Route = createFileRoute("/(auth)/two-factor")({
  validateSearch: authSearchSchema,
  beforeLoad: ({ context, search }) => {
    if (context.isAuthenticated) {
      throw redirect({ to: search.redirect ?? "/app" });
    }
  },
  head: () => ({ meta: [{ title: "Two-factor verification · Strait" }] }),
  errorComponent: ErrorComponent,
  notFoundComponent: NotFound,
  component: TwoFactorPage,
});

function TwoFactorPage() {
  const { redirect: redirectTo } = Route.useSearch();

  return (
    <AuthLayout title="Two-factor verification">
      <TwoFactorForm redirectTo={redirectTo} />
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
