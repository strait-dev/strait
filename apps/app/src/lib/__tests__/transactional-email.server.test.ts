import { beforeEach, describe, expect, it, vi } from "vitest";
import { handleTransactionalEmailRequest } from "@/lib/transactional-email.server";

const { captureExceptionMock, sendMock } = vi.hoisted(() => ({
  captureExceptionMock: vi.fn(),
  sendMock: vi.fn(),
}));

vi.mock("@/lib/resend.server", () => ({
  getResend: () => ({
    emails: {
      send: sendMock,
    },
  }),
}));

vi.mock("@/lib/sentry", () => ({
  captureException: captureExceptionMock,
}));

const request = (body: unknown, secret = "test-secret") =>
  new Request("http://localhost/internal/transactional-email", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      "X-Internal-Secret": secret,
    },
    body: JSON.stringify(body),
  });

describe("handleTransactionalEmailRequest", () => {
  beforeEach(() => {
    vi.stubEnv("INTERNAL_SECRET", "test-secret");
    vi.stubEnv("RESEND_FROM_EMAIL", "billing@strait.dev");
    captureExceptionMock.mockReset();
    sendMock.mockReset();
    sendMock.mockResolvedValue({ data: { id: "email-123" }, error: null });
  });

  it("rejects missing or invalid internal secrets", async () => {
    const response = await handleTransactionalEmailRequest(
      request(
        {
          template: "billing.payment_failed",
          to: ["admin@example.com"],
          idempotencyKey: "email-key",
          props: {},
        },
        "wrong-secret"
      )
    );

    expect(response.status).toBe(401);
    expect(sendMock).not.toHaveBeenCalled();
  });

  it("rejects invalid payloads", async () => {
    const response = await handleTransactionalEmailRequest(
      request({
        template: "billing.payment_failed",
        to: [],
        idempotencyKey: "",
        props: {},
      })
    );

    expect(response.status).toBe(400);
    expect(sendMock).not.toHaveBeenCalled();
  });

  it("rejects unknown templates", async () => {
    const response = await handleTransactionalEmailRequest(
      request({
        template: "billing.unknown",
        to: ["admin@example.com"],
        idempotencyKey: "email-key",
        props: {},
      })
    );

    expect(response.status).toBe(400);
    expect(sendMock).not.toHaveBeenCalled();
  });

  it("rejects invalid template props", async () => {
    const response = await handleTransactionalEmailRequest(
      request({
        template: "billing.payment_failed",
        to: ["admin@example.com"],
        idempotencyKey: "email-key",
        props: {
          gracePeriodEnd: "April 15, 2026",
          name: "",
        },
      })
    );

    expect(response.status).toBe(400);
    expect(sendMock).not.toHaveBeenCalled();
  });

  it("rejects extra template props", async () => {
    const response = await handleTransactionalEmailRequest(
      request({
        template: "billing.payment_failed",
        to: ["admin@example.com"],
        idempotencyKey: "email-key",
        props: {
          extra: true,
          gracePeriodEnd: "April 15, 2026",
          name: "",
          planName: "Pro",
        },
      })
    );

    expect(response.status).toBe(400);
    expect(sendMock).not.toHaveBeenCalled();
  });

  it("sends known templates through Resend with idempotency", async () => {
    const response = await handleTransactionalEmailRequest(
      request({
        template: "billing.payment_failed",
        to: ["admin@example.com"],
        idempotencyKey: "billing:payment_failed:org-1",
        props: {
          gracePeriodEnd: "April 15, 2026",
          name: "",
          planName: "Pro",
        },
      })
    );

    expect(response.status).toBe(200);
    expect(await response.json()).toEqual({ id: "email-123" });
    expect(sendMock).toHaveBeenCalledWith(
      expect.objectContaining({
        from: "billing@strait.dev",
        subject: "Action required: payment failed",
        to: ["admin@example.com"],
      }),
      { idempotencyKey: "billing:payment_failed:org-1" }
    );
  });

  it("passes base64 attachments to Resend", async () => {
    await handleTransactionalEmailRequest(
      request({
        template: "billing.usage_report",
        to: ["admin@example.com"],
        idempotencyKey: "billing:usage_report:org-1:2026-04-30",
        props: {
          addonCount: 1,
          orgId: "org-1",
          overageAmount: "$1.00",
          periodEnd: "Apr 30, 2026",
          periodStart: "Apr 1",
          planTier: "pro",
        },
        attachments: [
          {
            contentBase64: "cGRm",
            contentType: "application/pdf",
            filename: "usage.pdf",
          },
        ],
      })
    );

    expect(sendMock.mock.calls[0]?.[0].attachments).toEqual([
      {
        content: "cGRm",
        contentType: "application/pdf",
        filename: "usage.pdf",
      },
    ]);
  });

  it.each([
    [
      "invalid type",
      {
        contentBase64: "cGRm",
        contentType: "text/plain",
        filename: "usage.pdf",
      },
    ],
    [
      "invalid filename",
      {
        contentBase64: "cGRm",
        contentType: "application/pdf",
        filename: "../usage.pdf",
      },
    ],
    [
      "invalid base64",
      {
        contentBase64: "%%%%",
        contentType: "application/pdf",
        filename: "usage.pdf",
      },
    ],
    [
      "oversized attachment",
      {
        contentBase64: Buffer.alloc(10 * 1024 * 1024 + 1).toString("base64"),
        contentType: "application/pdf",
        filename: "usage.pdf",
      },
    ],
  ])("rejects %s attachments", async (_name, attachment) => {
    const response = await handleTransactionalEmailRequest(
      request({
        template: "billing.usage_report",
        to: ["admin@example.com"],
        idempotencyKey: "billing:usage_report:org-1:2026-04-30",
        props: {
          addonCount: 1,
          orgId: "org-1",
          overageAmount: "$1.00",
          periodEnd: "Apr 30, 2026",
          periodStart: "Apr 1",
          planTier: "pro",
        },
        attachments: [attachment],
      })
    );

    expect(response.status).toBe(400);
    expect(sendMock).not.toHaveBeenCalled();
  });

  it("rejects too many attachments", async () => {
    const response = await handleTransactionalEmailRequest(
      request({
        template: "billing.usage_report",
        to: ["admin@example.com"],
        idempotencyKey: "billing:usage_report:org-1:2026-04-30",
        props: {
          addonCount: 1,
          orgId: "org-1",
          overageAmount: "$1.00",
          periodEnd: "Apr 30, 2026",
          periodStart: "Apr 1",
          planTier: "pro",
        },
        attachments: Array.from({ length: 4 }, (_, index) => ({
          contentBase64: "cGRm",
          contentType: "application/pdf",
          filename: `usage-${index}.pdf`,
        })),
      })
    );

    expect(response.status).toBe(400);
    expect(sendMock).not.toHaveBeenCalled();
  });

  it("returns an error response when Resend fails", async () => {
    sendMock.mockResolvedValue({
      data: null,
      error: { message: "resend unavailable" },
    });

    const response = await handleTransactionalEmailRequest(
      request({
        template: "billing.payment_failed",
        to: ["admin@example.com"],
        idempotencyKey: "email-key",
        props: {
          gracePeriodEnd: "April 15, 2026",
          name: "",
          planName: "Pro",
        },
      })
    );

    expect(response.status).toBe(502);
    expect(captureExceptionMock).toHaveBeenCalled();
  });
});
