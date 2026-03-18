import Link from "next/link";

export default function NotFound() {
  return (
    <div className="flex min-h-[60vh] flex-col items-center justify-center px-6 text-center">
      <p className="font-mono text-muted-foreground text-sm">404</p>
      <h1 className="mt-4 font-bold text-3xl tracking-tight">Page not found</h1>
      <p className="mt-4 max-w-md text-muted-foreground">
        The page you are looking for does not exist or has been moved.
      </p>
      <div className="mt-8 flex gap-4">
        <Link
          href="/"
          className="inline-flex h-10 items-center rounded-lg bg-primary px-6 font-medium text-primary-foreground text-sm transition-colors hover:bg-primary/90"
        >
          Home
        </Link>
        <Link
          href="/docs/getting-started"
          className="inline-flex h-10 items-center rounded-lg border border-border px-6 font-medium text-sm transition-colors hover:bg-accent"
        >
          Documentation
        </Link>
      </div>
    </div>
  );
}
