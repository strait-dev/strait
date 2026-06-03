import { HugeiconsIcon } from "@hugeicons/react";
import { Button } from "@strait/ui/components/button";
import {
  Empty,
  EmptyContent,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from "@strait/ui/components/empty";
import { Link } from "@tanstack/react-router";
import { AlertIcon } from "@/lib/icons";

const NotFound = () => (
  <Empty border={false} className="min-h-[400px]">
    <EmptyHeader>
      <EmptyMedia media="icon" size="lg">
        <HugeiconsIcon className="size-6 text-foreground" icon={AlertIcon} />
      </EmptyMedia>
      <EmptyTitle>Page not found</EmptyTitle>
      <EmptyDescription>
        This URL doesn't match any page. It may have been moved or removed.
      </EmptyDescription>
    </EmptyHeader>
    <EmptyContent>
      <Button
        onClick={() => window.history.back()}
        type="button"
        variant="outline"
      >
        Go back
      </Button>
      <Button render={<Link preload="intent" to="/" />}>Home</Button>
    </EmptyContent>
  </Empty>
);

export default NotFound;
