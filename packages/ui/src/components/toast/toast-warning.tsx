import { AlertDiamondIcon } from "@hugeicons/core-free-icons";
import { HugeiconsIcon } from "@hugeicons/react";
import { ToastContent } from "./toast-content";
import type { ToastContentProps } from "./types";

export function ToastWarning(props: ToastContentProps) {
  return (
    <ToastContent
      {...props}
      icon={<HugeiconsIcon className="size-4" icon={AlertDiamondIcon} />}
      iconClassName="text-amber-500"
    />
  );
}
