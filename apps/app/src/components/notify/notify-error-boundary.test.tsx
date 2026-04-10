// @vitest-environment jsdom

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import NotifyErrorBoundary from "./notify-error-boundary";

const ThrowingChild = () => {
  throw new Error("render failure");
};

const wrapper = ({ children }: { children: React.ReactNode }) => (
  <QueryClientProvider client={new QueryClient()}>
    {children}
  </QueryClientProvider>
);

describe("NotifyErrorBoundary", () => {
  it("renders children when there is no error", () => {
    render(
      <NotifyErrorBoundary>
        <p>notify content</p>
      </NotifyErrorBoundary>,
      { wrapper }
    );

    expect(screen.getByText("notify content")).toBeTruthy();
  });

  it("renders the default fallback message when a child throws", () => {
    render(
      <NotifyErrorBoundary>
        <ThrowingChild />
      </NotifyErrorBoundary>,
      { wrapper }
    );

    expect(screen.getByText("Failed to load notify data")).toBeTruthy();
  });

  it("renders a custom fallback message when provided", () => {
    render(
      <NotifyErrorBoundary message="Could not load compose data">
        <ThrowingChild />
      </NotifyErrorBoundary>,
      { wrapper }
    );

    expect(screen.getByText("Could not load compose data")).toBeTruthy();
  });
});
