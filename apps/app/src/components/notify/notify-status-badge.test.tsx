// @vitest-environment jsdom

import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import NotifyStatusBadge from "./notify-status-badge";

describe("NotifyStatusBadge", () => {
  it("formats underscore statuses", () => {
    render(<NotifyStatusBadge status="in_progress" />);

    expect(screen.getByText("In Progress")).toBeTruthy();
  });

  it("renders known delivery status", () => {
    render(<NotifyStatusBadge status="delivered" />);

    expect(screen.getByText("Delivered")).toBeTruthy();
  });

  it("falls back to unknown label", () => {
    render(<NotifyStatusBadge status="" />);

    expect(screen.getByText("Unknown")).toBeTruthy();
  });
});
