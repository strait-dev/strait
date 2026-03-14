import { AlertDiamondIcon, Cancel01Icon } from "@hugeicons/core-free-icons";
import { HugeiconsIcon } from "@hugeicons/react";
import { Button } from "../button";
import type { ConfirmToastOptions } from "./types";

interface ToastConfirmProps extends ConfirmToastOptions {
  onDismiss: () => void;
  title: string;
}

export function ToastConfirm({
  title,
  description,
  confirmLabel = "Confirm",
  cancelLabel = "Cancel",
  onConfirm,
  onCancel,
  onError,
  onDismiss,
  variant = "default",
}: ToastConfirmProps) {
  const handleCancel = () => {
    onCancel?.();
    onDismiss();
  };

  const handleConfirm = async () => {
    try {
      await onConfirm();
    } catch (error) {
      if (onError) {
        onError(error);
      } else {
        console.error("Confirm toast error:", error);
      }
    } finally {
      onDismiss();
    }
  };

  return (
    <div className="flex w-full max-w-[400px] flex-col gap-3 rounded-md border border-border bg-background p-3 shadow-black/5 shadow-lg">
      <div className="flex items-start gap-3">
        <div
          className={`mt-0.5 shrink-0 ${variant === "destructive" ? "text-destructive" : "text-amber-500"}`}
        >
          <HugeiconsIcon className="size-4" icon={AlertDiamondIcon} />
        </div>

        <div className="flex min-w-0 flex-1 flex-col">
          <p className="font-medium text-[13px] text-foreground">{title}</p>
          {description ? (
            <p className="text-muted-foreground text-xs">{description}</p>
          ) : null}
        </div>

        <button
          className="shrink-0 rounded-md p-1 text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
          onClick={(e) => {
            e.stopPropagation();
            handleCancel();
          }}
          type="button"
        >
          <HugeiconsIcon className="size-3.5" icon={Cancel01Icon} />
        </button>
      </div>

      <div className="flex items-center justify-end gap-2">
        <Button onClick={handleCancel} size="sm" variant="outline">
          {cancelLabel}
        </Button>
        <Button
          onClick={handleConfirm}
          size="sm"
          variant={variant === "destructive" ? "destructive" : "default"}
        >
          {confirmLabel}
        </Button>
      </div>
    </div>
  );
}
