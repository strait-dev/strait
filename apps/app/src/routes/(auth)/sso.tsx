import { createFileRoute, Link, redirect } from "@tanstack/react-router";
import AuthLayout from "@/components/(auth)/auth-layout";
import SsoForm from "@/components/(auth)/sso-form";
import ErrorComponent from "@/components/common/error-component";
import NotFound from "@/components/common/not-found";
import { authSearchSchema } from "@/lib/auth-search-schema";

export const Route = createFileRoute("/(auth)/sso")({
  validateSearch: authSearchSchema,
  beforeLoad: ({ context, search }) => {
    if (context.isAuthenticated) {
      throw redirect({ to: search.redirect ?? "/app" });
    }
  },
  head: () => ({ meta: [{ title: "SSO · Strait" }] }),
  errorComponent: ErrorComponent,
  notFoundComponent: NotFound,
  component: SsoPage,
});

function SsoPage() {
  const { redirect: redirectTo } = Route.useSearch();

  return (
    <AuthLayout title="SSO roadmap">
      <SsoForm redirectTo={redirectTo} />
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
