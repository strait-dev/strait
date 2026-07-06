import { createFileRoute, Link, redirect } from "@tanstack/react-router";
import AuthLayout from "@/components/(auth)/auth-layout";
import MagicLinkForm from "@/components/(auth)/magic-link-form";
import ErrorComponent from "@/components/common/error-component";
import NotFound from "@/components/common/not-found";
import { authSearchSchema } from "@/lib/auth-search-schema";
import { seo } from "@/lib/seo";

export const Route = createFileRoute("/(auth)/magic-link")({
  validateSearch: authSearchSchema,
  beforeLoad: ({ context, search }) => {
    if (context.isAuthenticated) {
      throw redirect({ to: search.redirect ?? "/app" });
    }
  },
  head: () => ({ meta: seo({ title: "Magic link sign in" }) }),
  errorComponent: ErrorComponent,
  notFoundComponent: NotFound,
  component: MagicLinkPage,
});

function MagicLinkPage() {
  const { redirect: redirectTo } = Route.useSearch();

  return (
    <AuthLayout title="Magic link sign in">
      <MagicLinkForm redirectTo={redirectTo} />
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
