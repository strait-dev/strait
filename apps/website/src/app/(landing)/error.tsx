"use client";

import { Button } from "@strait/ui/components/button";
import Link from "next/link";

import Shell from "@/components/layout/shell";

const ErrorPage = ({
  reset,
}: {
  error: globalThis.Error & { digest?: string };
  reset: () => void;
}) => (
  <main className="flex min-h-[60vh] items-center justify-center py-20 sm:py-28">
    <Shell variant="wide">
      <div className="mx-auto max-w-md text-center">
        <p className="font-medium text-muted-foreground text-sm uppercase tracking-wider">
          Something went wrong
        </p>
        <h1 className="mt-4 font-bold text-3xl text-foreground tracking-tight sm:text-4xl">
          Unexpected Error
        </h1>
        <p className="mt-4 text-base text-muted-foreground leading-relaxed">
          We couldn&apos;t load this page. This is usually temporary — try
          refreshing, or head back to the homepage.
        </p>
        <div className="mt-8 flex items-center justify-center gap-3">
          <Button onClick={() => reset()} type="button" variant="default">
            Try again
          </Button>
          <Button render={<Link href="/" />} variant="outline">
            Go home
          </Button>
        </div>
      </div>
    </Shell>
  </main>
);

export default ErrorPage;
