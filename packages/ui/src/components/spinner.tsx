import { Loading03Icon } from "@hugeicons/core-free-icons";
import { HugeiconsIcon } from "@hugeicons/react";
import { cn } from "../utils/index";

function Spinner({
  className,
  strokeWidth = 2,
  ...props
}: React.ComponentProps<"svg">) {
  return (
    <HugeiconsIcon
      aria-label="Loading"
      className={cn("size-4 animate-spin", className)}
      icon={Loading03Icon}
      role="status"
      strokeWidth={Number(strokeWidth)}
      {...props}
    />
  );
}

export { Spinner };
