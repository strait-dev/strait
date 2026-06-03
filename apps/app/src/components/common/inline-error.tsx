import { HugeiconsIcon } from "@hugeicons/react";
import { Alert, AlertDescription } from "@strait/ui/components/alert";
import { Button } from "@strait/ui/components/button";
import { AlertIcon } from "@/lib/icons";

type InlineErrorProps = {
  message?: string;
  onRetry?: () => void;
};

const InlineError = ({
  message = "Failed to load",
  onRetry,
}: InlineErrorProps) => (
  <Alert variant="destructive">
    <HugeiconsIcon className="size-4" icon={AlertIcon} />
    <AlertDescription className="flex items-center justify-between gap-3">
      <span>{message}</span>
      {onRetry && (
        <Button onClick={onRetry} variant="outline">
          Retry
        </Button>
      )}
    </AlertDescription>
  </Alert>
);

export default InlineError;
