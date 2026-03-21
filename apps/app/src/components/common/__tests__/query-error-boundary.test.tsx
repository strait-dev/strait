// @ts-nocheck
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import InlineError from "../inline-error";
import { QueryErrorBoundary } from "../query-error-boundary";

afterEach(cleanup);

function createWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return ({ children }: { children: React.ReactNode }) => (
    <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  );
}

function ThrowingChild({ shouldThrow }: { shouldThrow: boolean }) {
  if (shouldThrow) {
    throw new Error("Test error");
  }
  return <div data-testid="child">Child content</div>;
}

describe("QueryErrorBoundary", () => {
  it("renders children when no error is thrown", () => {
    render(
      <QueryErrorBoundary
        fallback={({ error, resetErrorBoundary }) => (
          <InlineError message={error.message} onRetry={resetErrorBoundary} />
        )}
      >
        <ThrowingChild shouldThrow={false} />
      </QueryErrorBoundary>,
      { wrapper: createWrapper() }
    );

    expect(screen.getByTestId("child")).toBeDefined();
    expect(screen.getByText("Child content")).toBeDefined();
  });

  it("renders fallback when child throws an error", () => {
    // Suppress React's error boundary console.error in test output
    const consoleSpy = vi.spyOn(console, "error").mockImplementation(vi.fn());

    render(
      <QueryErrorBoundary
        fallback={({ error, resetErrorBoundary }) => (
          <InlineError message={error.message} onRetry={resetErrorBoundary} />
        )}
      >
        <ThrowingChild shouldThrow={true} />
      </QueryErrorBoundary>,
      { wrapper: createWrapper() }
    );

    expect(screen.getByText("Test error")).toBeDefined();
    expect(screen.getByRole("button", { name: "Retry" })).toBeDefined();

    consoleSpy.mockRestore();
  });

  it("resets error boundary when retry is clicked", () => {
    const consoleSpy = vi.spyOn(console, "error").mockImplementation(vi.fn());
    let shouldThrow = true;

    function ConditionalThrower() {
      if (shouldThrow) {
        throw new Error("Transient error");
      }
      return <div data-testid="recovered">Recovered</div>;
    }

    render(
      <QueryErrorBoundary
        fallback={({ error, resetErrorBoundary }) => (
          <InlineError message={error.message} onRetry={resetErrorBoundary} />
        )}
      >
        <ConditionalThrower />
      </QueryErrorBoundary>,
      { wrapper: createWrapper() }
    );

    // Should show error state
    expect(screen.getByText("Transient error")).toBeDefined();

    // Fix the error condition and retry
    shouldThrow = false;
    fireEvent.click(screen.getByRole("button", { name: "Retry" }));

    // Should show recovered content
    expect(screen.getByTestId("recovered")).toBeDefined();
    expect(screen.getByText("Recovered")).toBeDefined();

    consoleSpy.mockRestore();
  });

  it("renders custom fallback content", () => {
    const consoleSpy = vi.spyOn(console, "error").mockImplementation(vi.fn());

    render(
      <QueryErrorBoundary
        fallback={({ error }) => (
          <div data-testid="custom-fallback">Custom: {error.message}</div>
        )}
      >
        <ThrowingChild shouldThrow={true} />
      </QueryErrorBoundary>,
      { wrapper: createWrapper() }
    );

    expect(screen.getByTestId("custom-fallback")).toBeDefined();
    expect(screen.getByText("Custom: Test error")).toBeDefined();

    consoleSpy.mockRestore();
  });
});

describe("InlineError", () => {
  it("renders default message", () => {
    render(<InlineError />);
    expect(screen.getByText("Failed to load")).toBeDefined();
  });

  it("renders custom message", () => {
    render(<InlineError message="Could not load chart data" />);
    expect(screen.getByText("Could not load chart data")).toBeDefined();
  });

  it("renders retry button when onRetry is provided", () => {
    const onRetry = vi.fn();
    render(<InlineError onRetry={onRetry} />);

    const retryButton = screen.getByRole("button", { name: "Retry" });
    expect(retryButton).toBeDefined();

    fireEvent.click(retryButton);
    expect(onRetry).toHaveBeenCalledOnce();
  });

  it("does not render retry button when onRetry is not provided", () => {
    render(<InlineError />);
    expect(screen.queryByRole("button", { name: "Retry" })).toBeNull();
  });
});
