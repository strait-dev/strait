import { Loading03Icon } from "@hugeicons/core-free-icons";
import { HugeiconsIcon } from "@hugeicons/react";
import { cn } from "../utils/index.ts";

function Spinner({
  className,
  ...props
}: Omit<React.ComponentProps<typeof HugeiconsIcon>, "icon">) {
  return (
    <HugeiconsIcon
      aria-label="Loading"
      className={cn("size-4 animate-spin", className)}
      icon={Loading03Icon}
      role="status"
      strokeWidth={2}
      {...props}
    />
  );
}

export { Spinner };
