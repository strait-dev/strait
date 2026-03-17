import { HugeiconsIcon } from "@hugeicons/react";
import { Button } from "@strait/ui/components/button";
import { Link } from "@tanstack/react-router";
import { SearchIcon } from "@/lib/icons";

type EntityNotFoundProps = {
  entity: string;
  backTo?: string;
  backLabel?: string;
};

const EntityNotFound = ({ entity, backTo, backLabel }: EntityNotFoundProps) => {
  const back = backTo ?? "/app";
  const label = backLabel ?? `Back to ${entity}s`;

  return (
    <div className="flex min-h-[350px] flex-col items-center justify-center gap-4 p-8 text-center">
      <div className="flex size-14 items-center justify-center rounded-xl bg-muted/70">
        <HugeiconsIcon
          className="size-6 text-muted-foreground"
          icon={SearchIcon}
        />
      </div>
      <div className="space-y-1.5">
        <h2 className="font-medium text-base text-secondary-foreground">
          {entity} not found
        </h2>
        <p className="max-w-sm text-pretty text-muted-foreground text-sm">
          The {entity.toLowerCase()} you're looking for doesn't exist or may
          have been removed.
        </p>
      </div>
      <div className="flex items-center gap-2 pt-1">
        <Button
          onClick={() => window.history.back()}
          size="sm"
          type="button"
          variant="outline"
        >
          Go back
        </Button>
        <Button render={<Link to={back} />} size="sm">
          {label}
        </Button>
      </div>
    </div>
  );
};

export default EntityNotFound;
