import { Alert02Icon } from "@hugeicons/core-free-icons";
import { HugeiconsIcon } from "@hugeicons/react";
import { ToastContent } from "./toast-content";
import type { ToastContentProps } from "./types";

export function ToastError(props: ToastContentProps) {
  return (
    <ToastContent
      {...props}
      copyable
      icon={<HugeiconsIcon className="size-4" icon={Alert02Icon} />}
      iconClassName="text-destructive"
    />
  );
}
