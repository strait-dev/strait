import { describe, expect, it } from "vitest";
import type {
  NotifyMessageStatus,
  NotifySubscriberStatus,
} from "@/hooks/api/types";
import {
  NOTIFY_DELIVERY_STATUS_OPTIONS,
  NOTIFY_SUBSCRIBER_STATUS_OPTIONS,
} from "./status";

describe("notify status options", () => {
  it("matches supported delivery statuses", () => {
    const expected: NotifyMessageStatus[] = [
      "rendering",
      "scheduled",
      "pending",
      "processing",
      "delivered",
      "failed",
      "bounced",
      "cancelled",
    ];

    expect(NOTIFY_DELIVERY_STATUS_OPTIONS).toEqual(expected);
  });

  it("matches supported subscriber statuses", () => {
    const expected: NotifySubscriberStatus[] = [
      "active",
      "unsubscribed",
      "deleted",
    ];

    expect(NOTIFY_SUBSCRIBER_STATUS_OPTIONS).toEqual(expected);
  });
});
