"use client";

import { cn } from "@strait/ui/utils";
import { GeistSans } from "geist/font/sans";

import ErrorDisplay from "@/components/error-display.tsx";

const GlobalError = ({
  reset,
}: {
  error: Error & { digest?: string };
  reset: () => void;
}) => (
  <html
    className={cn(
      "min-h-screen bg-background antialiased",
      GeistSans.className
    )}
    lang="en-US"
  >
    <body>
      <ErrorDisplay
        actions={[
          { label: "Try again", onClick: () => reset(), variant: "default" },
        ]}
        description="We hit an unexpected issue. Please try again, and if the problem persists, reach out to our support team."
        title="Unexpected error"
      />
    </body>
  </html>
);

export default GlobalError;
