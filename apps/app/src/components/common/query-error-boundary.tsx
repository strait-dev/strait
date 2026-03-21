import { QueryErrorResetBoundary } from "@tanstack/react-query";
import { Component, type ReactNode } from "react";

type FallbackProps = {
  error: Error;
  resetErrorBoundary: () => void;
};

type QueryErrorBoundaryProps = {
  children: ReactNode;
  fallback: (props: FallbackProps) => ReactNode;
};

type State = {
  error: Error | null;
};

// biome-ignore lint/style/useReactFunctionComponents: React error boundaries require class components — no hook equivalent exists
class ErrorBoundaryInner extends Component<
  QueryErrorBoundaryProps & { onReset: () => void },
  State
> {
  state: State = { error: null };

  static getDerivedStateFromError(error: Error): State {
    return { error };
  }

  reset = () => {
    this.props.onReset();
    this.setState({ error: null });
  };

  render() {
    if (this.state.error) {
      return this.props.fallback({
        error: this.state.error,
        resetErrorBoundary: this.reset,
      });
    }
    return this.props.children;
  }
}

export const QueryErrorBoundary = ({
  children,
  fallback,
}: QueryErrorBoundaryProps) => {
  return (
    <QueryErrorResetBoundary>
      {({ reset }) => (
        <ErrorBoundaryInner fallback={fallback} onReset={reset}>
          {children}
        </ErrorBoundaryInner>
      )}
    </QueryErrorResetBoundary>
  );
};

export type { FallbackProps };
