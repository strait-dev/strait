import { HugeiconsIcon } from "@hugeicons/react";
import { Button } from "@strait/ui/components/button";
import { Link } from "@tanstack/react-router";
import { AlertIcon } from "@/lib/icons";

const NotFound = () => (
  <div className="flex min-h-[400px] flex-col items-center justify-center gap-4 p-8 text-center">
    <div className="flex size-14 items-center justify-center rounded-xl bg-muted/70">
      <HugeiconsIcon
        className="size-6 text-muted-foreground"
        icon={AlertIcon}
      />
    </div>
    <div className="space-y-2">
      <h2 className="text-balance font-normal text-lg text-secondary-foreground tracking-tight">
        Page not found
      </h2>
      <p className="text-pretty text-muted-foreground text-sm">
        This URL doesn't match any page. It may have been moved or removed.
      </p>
    </div>
    <div className="flex items-center gap-2">
      <Button
        onClick={() => window.history.back()}
        type="button"
        variant="outline"
      >
        Go back
      </Button>
      <Button render={<Link preload="intent" to="/" />}>Home</Button>
    </div>
  </div>
);

export default NotFound;
