import { describe, expect, it } from "vitest";
import type {
  NotificationMessage,
  NotificationProvider,
  NotifyPolicyOverride,
  NotifySubscriber,
} from "@/hooks/api/types";
import { buildNotifyOpsSnapshot } from "./notify-ops";

const delivery = (
  status: string,
  suppressionReason?: string
): NotificationMessage => ({
  id: crypto.randomUUID(),
  project_id: "project_123",
  recipient_type: "subscriber",
  recipient_id: "sub_123",
  channel: "email",
  status,
  attempts: 1,
  suppression_reason: suppressionReason,
  created_at: "2026-04-08T10:00:00Z",
});

const subscriber = (status = "active"): NotifySubscriber => ({
  id: crypto.randomUUID(),
  project_id: "project_123",
  external_id: "ext_123",
  locale: "en",
  timezone: "UTC",
  status,
  created_at: "2026-04-08T10:00:00Z",
  updated_at: "2026-04-08T10:00:00Z",
});

const provider = (
  options?: Partial<NotificationProvider>
): NotificationProvider => ({
  id: crypto.randomUUID(),
  project_id: "project_123",
  channel: "email",
  provider: "ses",
  name: "SES Default",
  is_default: true,
  health: "healthy",
  created_at: "2026-04-08T10:00:00Z",
  updated_at: "2026-04-08T10:00:00Z",
  ...options,
});

const policy = (): NotifyPolicyOverride => ({
  id: crypto.randomUUID(),
  project_id: "project_123",
  scope_type: "project",
  scope_key: "project",
  enabled: true,
  created_at: "2026-04-08T10:00:00Z",
  updated_at: "2026-04-08T10:00:00Z",
});

describe("buildNotifyOpsSnapshot", () => {
  it("reports healthy when checks pass", () => {
    const snapshot = buildNotifyOpsSnapshot({
      deliveries: [delivery("delivered"), delivery("processing")],
      subscribers: [subscriber("active")],
      providers: [provider()],
      policies: [policy()],
    });

    expect(snapshot.health).toBe("healthy");
    expect(snapshot.errorRate).toBe(0);
    expect(snapshot.reasons).toEqual([
      "All notify operational checks are healthy.",
    ]);
  });

  it("reports degraded for medium error rate and unhealthy provider", () => {
    const snapshot = buildNotifyOpsSnapshot({
      deliveries: [
        ...Array.from({ length: 18 }, () => delivery("delivered")),
        delivery("failed"),
        delivery("bounced"),
      ],
      subscribers: [subscriber("active"), subscriber("active")],
      providers: [provider({ health: "degraded" })],
      policies: [],
    });

    expect(snapshot.health).toBe("degraded");
    expect(snapshot.errorRate).toBe(0.1);
    expect(snapshot.reasons).toContain(
      "Delivery error rate is above 5% in the current window."
    );
    expect(snapshot.reasons).toContain(
      "At least one provider is reporting non-healthy state."
    );
  });

  it("reports critical when no default provider or high error rate", () => {
    const snapshot = buildNotifyOpsSnapshot({
      deliveries: [
        ...Array.from({ length: 6 }, () => delivery("failed")),
        ...Array.from({ length: 4 }, () => delivery("delivered")),
      ],
      subscribers: [
        ...Array.from({ length: 6 }, () => subscriber("inactive")),
        ...Array.from({ length: 4 }, () => subscriber("active")),
      ],
      providers: [provider({ is_default: false, health: "down" })],
      policies: [],
    });

    expect(snapshot.health).toBe("critical");
    expect(snapshot.reasons).toContain(
      "No default email provider is configured."
    );
    expect(snapshot.reasons).toContain(
      "Delivery error rate is above 20% in the current window."
    );
    expect(snapshot.reasons).toContain(
      "More than half of subscribers are not active."
    );
  });
});
