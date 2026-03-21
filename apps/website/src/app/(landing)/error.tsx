"use client";

import ErrorDisplay from "@/components/error-display.tsx";

const ErrorPage = ({
  reset,
}: {
  error: globalThis.Error & { digest?: string };
  reset: () => void;
}) => (
  <main>
    <ErrorDisplay
      actions={[
        { label: "Try again", onClick: () => reset(), variant: "default" },
        { label: "Go home", href: "/", variant: "outline" },
      ]}
      description="We couldn't load this page. This is usually temporary — try refreshing, or head back to the homepage."
      title="Unexpected error"
    />
  </main>
);

export default ErrorPage;
