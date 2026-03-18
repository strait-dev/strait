import {
  Cancel01Icon,
  Copy01Icon,
  Tick01Icon,
} from "@hugeicons/core-free-icons";
import { HugeiconsIcon } from "@hugeicons/react";
import { type ReactNode, useState } from "react";
import { cn } from "../../utils/index";
import type { ToastContentProps } from "./types";

interface ToastContentInternalProps extends ToastContentProps {
  icon: ReactNode;
  iconClassName?: string;
}

function DismissButton({ onDismiss }: { onDismiss: () => void }) {
  return (
    <button
      aria-label="Dismiss"
      className="rounded-md p-1 text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
      onClick={(e) => {
        e.stopPropagation();
        onDismiss();
      }}
      type="button"
    >
      <HugeiconsIcon className="size-3.5" icon={Cancel01Icon} />
    </button>
  );
}

function CopyButton({ text }: { text: string }) {
  const [copied, setCopied] = useState(false);

  return (
    <button
      aria-label="Copy error"
      className="rounded-md p-1 text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
      onClick={(e) => {
        e.stopPropagation();
        navigator.clipboard.writeText(text);
        setCopied(true);
        setTimeout(() => setCopied(false), 1500);
      }}
      type="button"
    >
      <HugeiconsIcon
        className="size-3.5"
        icon={copied ? Tick01Icon : Copy01Icon}
      />
    </button>
  );
}

export function ToastContent({
  title,
  description,
  action,
  onDismiss,
  dismissible = true,
  copyable = false,
  icon,
  iconClassName,
  className,
}: ToastContentInternalProps) {
  const showDismissButton = dismissible && onDismiss;

  const copyText = description ? `${title}\n${description}` : title;

  return (
    <div
      className={cn(
        "flex w-full max-w-[400px] items-center gap-3 rounded-md border border-border bg-background p-3 shadow-black/5 shadow-lg",
        className
      )}
    >
      <div className={cn("shrink-0", iconClassName)}>{icon}</div>

      <div className="flex min-w-0 flex-1 flex-col">
        <p className="font-medium text-[13px] text-foreground">{title}</p>
        {description ? (
          <p className="text-muted-foreground text-xs">{description}</p>
        ) : null}
      </div>

      <div className="flex shrink-0 items-center gap-1">
        {action ? (
          <button
            className="h-6 rounded-md border border-input bg-background px-2 font-medium text-accent-foreground text-xs transition-colors hover:bg-accent"
            onClick={(e) => {
              e.stopPropagation();
              action.onClick();
              onDismiss?.();
            }}
            type="button"
          >
            {action.label}
          </button>
        ) : null}

        {copyable ? <CopyButton text={copyText} /> : null}

        {showDismissButton ? <DismissButton onDismiss={onDismiss} /> : null}
      </div>
    </div>
  );
}
