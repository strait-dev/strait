import { Loading03Icon } from "@hugeicons/core-free-icons";
import { HugeiconsIcon } from "@hugeicons/react";
import { Button } from "@strait/ui/components/button.tsx";
import { useNavigate } from "@tanstack/react-router";
import type { ReactNode } from "react";

type FloatingFooterProps = {
  to: string;
  isPending: boolean;
  onSubmit: () => void;
  onCancel: () => void;
  isSubmitDisabled: boolean;
  isSubmitting: boolean;
  cancelButtonText?: string;
  submitButtonText?: string;
  submitButtonIcon?: ReactNode;
  hideCancel?: boolean;
};

const FloatingFooter = ({
  to,
  isPending,
  onSubmit,
  onCancel,
  isSubmitDisabled,
  isSubmitting,
  cancelButtonText = "Cancel",
  submitButtonText = "Save",
  submitButtonIcon,
  hideCancel = false,
}: FloatingFooterProps) => {
  const navigate = useNavigate();

  const handleCancel = () => {
    if (onCancel) {
      onCancel();
    } else if (!isPending) {
      navigate({ to });
    }
  };

  return (
    <div className="fixed right-0 bottom-0 left-[var(--sidebar-width,0px)] z-[25] h-[64px] border-sidebar-border border-t bg-background shadow-md">
      <div className="mx-auto flex h-full w-full max-w-[1800px] items-center justify-end gap-2 px-4 sm:px-8 lg:px-20">
        {!hideCancel && (
          <Button
            disabled={isPending}
            onClick={handleCancel}
            type="button"
            variant="secondary"
          >
            {cancelButtonText}
          </Button>
        )}

        <Button disabled={isSubmitDisabled} onClick={onSubmit} type="submit">
          {(() => {
            if (isSubmitting) {
              return (
                <HugeiconsIcon
                  className="size-4 animate-spin"
                  icon={Loading03Icon}
                />
              );
            }
            if (submitButtonIcon) {
              return <span>{submitButtonIcon}</span>;
            }
            return null;
          })()}
          {submitButtonText}
        </Button>
      </div>
    </div>
  );
};

export default FloatingFooter;
