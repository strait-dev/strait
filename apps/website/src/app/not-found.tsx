import { Button } from "@strait/ui/components/button";
import type { Metadata } from "next";
import Link from "next/link";

import Shell from "@/components/layout/shell.tsx";
import Footer from "./(landing)/components/common/footer/footer.tsx";
import Header from "./(landing)/components/common/header/header.tsx";

export const metadata: Metadata = {
  title: "Page Not Found — Strait",
  description: "The page you're looking for doesn't exist or has been moved.",
};

const NotFound = () => (
  <>
    <Header />
    <main className="flex min-h-[60vh] items-center justify-center py-20 sm:py-28">
      <Shell variant="wide">
        <div className="mx-auto max-w-md text-center">
          <p className="text-7xl text-foreground tracking-tight sm:text-8xl">
            404
          </p>
          <h1 className="mt-4 text-3xl text-foreground tracking-tight sm:text-4xl">
            Page not found
          </h1>
          <p className="mt-4 text-base text-muted-foreground leading-relaxed">
            The page you&apos;re looking for doesn&apos;t exist or has been
            moved. Check the URL, or head back to explore Strait.
          </p>
          <div className="mt-8 flex items-center justify-center gap-3">
            <Button render={<Link href="/" />} variant="default">
              Go home
            </Button>
            <Button render={<Link href="/blog" />} variant="outline">
              Read the blog
            </Button>
          </div>
        </div>
      </Shell>
    </main>
    <Footer />
  </>
);

export default NotFound;
