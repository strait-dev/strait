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
    <Empty border={false} className="min-h-[350px]">
      <EmptyHeader>
        <EmptyMedia media="icon" size="lg">
          <HugeiconsIcon className="size-6 text-foreground" icon={SearchIcon} />
        </EmptyMedia>
        <EmptyTitle>{entity} not found</EmptyTitle>
        <EmptyDescription>
          This {entity.toLowerCase()} doesn't exist or was removed. Check the
          URL or try searching.
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
        <Button render={<Link to={back} />}>{label}</Button>
      </EmptyContent>
    </Empty>
  );
};

export default EntityNotFound;
