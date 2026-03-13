"use client";

import { Button } from "@strait/ui/components/button";
import { cn } from "@strait/ui/utils";
import { GeistSans } from "geist/font/sans";

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
    <body className="flex min-h-screen items-center justify-center px-4">
      <div className="mx-auto max-w-md text-center">
        <p className="font-medium text-muted-foreground text-sm uppercase tracking-wider">
          Something went wrong
        </p>
        <h1 className="mt-4 text-3xl text-foreground tracking-tight sm:text-4xl">
          Unexpected Error
        </h1>
        <p className="mt-4 text-base text-muted-foreground leading-relaxed">
          We hit an unexpected issue. Please try again, and if the problem
          persists, reach out to our support team.
        </p>
        <Button className="mt-8" onClick={() => reset()} type="button">
          Try again
        </Button>
      </div>
    </body>
  </html>
);

export default GlobalError;
