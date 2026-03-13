import { CheckmarkCircle02Icon } from "@hugeicons/core-free-icons";
import { HugeiconsIcon } from "@hugeicons/react";
import { ToastContent } from "./toast-content.tsx";
import type { ToastContentProps } from "./types.ts";

export function ToastSuccess(props: ToastContentProps) {
  return (
    <ToastContent
      {...props}
      icon={<HugeiconsIcon className="size-4" icon={CheckmarkCircle02Icon} />}
      iconClassName="text-emerald-500"
    />
  );
}
