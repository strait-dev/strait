import { HugeiconsIcon } from "@hugeicons/react";
import { Button } from "@strait/ui/components/button";
import { AlertIcon } from "@/lib/icons";

type InlineErrorProps = {
  message?: string;
  onRetry?: () => void;
};

const InlineError = ({
  message = "Failed to load",
  onRetry,
}: InlineErrorProps) => {
  return (
    <div className="flex items-center justify-center gap-3 rounded-lg border border-border bg-muted/50 px-4 py-3">
      <HugeiconsIcon
        className="size-4 shrink-0 text-destructive"
        icon={AlertIcon}
      />
      <p className="text-muted-foreground text-sm">{message}</p>
      {onRetry && (
        <Button onClick={onRetry} size="sm" variant="ghost">
          Retry
        </Button>
      )}
    </div>
  );
};

export default InlineError;
