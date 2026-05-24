import { describe, expect, it } from "vitest";
import { scrubSentryBreadcrumb, scrubSentryEvent } from "../sentry-scrub";

describe("scrubSentryEvent", () => {
  it("removes sensitive URL params, headers, and user PII", () => {
    const event = scrubSentryEvent({
      request: {
        url: "https://app.strait.dev/reset-password?token=secret&next=/app",
        headers: {
          cookie: "session=secret",
          authorization: "Bearer secret",
          "x-safe": "ok",
        },
      },
      user: {
        id: "user-1",
        email: "person@example.com",
        username: "Person",
        ip_address: "127.0.0.1",
      },
    });

    expect(event).toMatchObject({
      request: {
        url: "https://app.strait.dev/reset-password?token=%5BFiltered%5D&next=%2Fapp",
        headers: {
          cookie: "[Filtered]",
          authorization: "[Filtered]",
          "x-safe": "ok",
        },
      },
      user: { id: "user-1" },
    });
  });
});

describe("scrubSentryBreadcrumb", () => {
  it("scrubs token-bearing breadcrumb URLs", () => {
    const breadcrumb = scrubSentryBreadcrumb({
      message: "GET /verify-email?token=secret",
      data: {
        url: "/oauth/consent?code=abc&state=xyz",
        headers: { cookie: "session=secret" },
      },
    });

    expect(breadcrumb?.message).toBe("GET /verify-email?token=[Filtered]");
    expect(breadcrumb?.data?.url).toBe(
      "/oauth/consent?code=%5BFiltered%5D&state=%5BFiltered%5D"
    );
    expect(breadcrumb?.data?.headers).toEqual({ cookie: "[Filtered]" });
  });
});
