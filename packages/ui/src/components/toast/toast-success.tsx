import { CheckmarkCircle02Icon } from "@hugeicons/core-free-icons";
import { HugeiconsIcon } from "@hugeicons/react";
import { ToastContent } from "./toast-content";
import type { ToastContentProps } from "./types";

export function ToastSuccess(props: ToastContentProps) {
  return (
    <ToastContent
      {...props}
      icon={<HugeiconsIcon className="size-4" icon={CheckmarkCircle02Icon} />}
      iconClassName="text-emerald-500"
    />
  );
}
