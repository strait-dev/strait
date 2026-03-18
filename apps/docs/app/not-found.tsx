import Link from "next/link";
import { Button } from "@strait/ui/components/button";
import { Badge } from "@strait/ui/components/badge";

export default function NotFound() {
  return (
    <div className="flex min-h-[60vh] flex-col items-center justify-center px-6 text-center">
      <Badge variant="secondary" className="font-mono">
        404
      </Badge>
      <h1 className="mt-4 font-bold text-3xl tracking-tight">
        Page not found
      </h1>
      <p className="mt-4 max-w-md text-muted-foreground">
        The page you are looking for does not exist or has been moved.
      </p>
      <div className="mt-8 flex gap-4">
        <Button size="lg" render={<Link href="/" />}>
          Home
        </Button>
        <Button variant="outline" size="lg" render={<Link href="/docs/getting-started" />}>
          Documentation
        </Button>
      </div>
    </div>
  );
}
