import type { ReactNode } from "react";
import InlineError from "@/components/common/inline-error";
import { QueryErrorBoundary } from "@/components/common/query-error-boundary";

type Props = {
  children: ReactNode;
  message?: string;
};

const NotifyErrorBoundary = ({
  children,
  message = "Failed to load notify data",
}: Props) => {
  return (
    <QueryErrorBoundary
      fallback={({ resetErrorBoundary }) => (
        <InlineError message={message} onRetry={resetErrorBoundary} />
      )}
    >
      {children}
    </QueryErrorBoundary>
  );
};

export default NotifyErrorBoundary;
