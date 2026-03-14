import { Loading03Icon } from "@hugeicons/core-free-icons";
import { HugeiconsIcon } from "@hugeicons/react";
import { ToastContent } from "./toast-content";
import type { ToastContentProps } from "./types";

export function ToastLoading(
  props: Omit<ToastContentProps, "action" | "dismissible">
) {
  return (
    <ToastContent
      {...props}
      dismissible={false}
      icon={
        <HugeiconsIcon className="size-4 animate-spin" icon={Loading03Icon} />
      }
      iconClassName="text-muted-foreground"
    />
  );
}
