import { describe, expect, it } from "bun:test";
import { Effect } from "effect";
import { makeInboxClient } from "./client";
import { InboxClientError } from "./errors";

describe("makeInboxClient", () => {
  it("sends subscriber auth headers and list query params", async () => {
    let capturedURL = "";
    let capturedMethod = "";
    let capturedAuth = "";

    const client = makeInboxClient({
      baseUrl: "https://api.strait.dev",
      token: "token_123",
      fetch: (input, init) => {
        capturedURL = String(input);
        capturedMethod = init?.method ?? "GET";
        capturedAuth = new Headers(init?.headers).get("Authorization") ?? "";
        return Promise.resolve(new Response("[]", { status: 200 }));
      },
    });

    const out = await Effect.runPromise(
      client.listInbox({
        limit: 20,
        state: "unread",
        cursor: "2026-04-05T11:00:00Z",
      })
    );

    expect(out).toEqual([]);
    expect(capturedMethod).toBe("GET");
    expect(capturedAuth).toBe("Bearer token_123");
    expect(capturedURL).toContain("/v1/inbox");
    expect(capturedURL).toContain("limit=20");
    expect(capturedURL).toContain("state=unread");
    expect(capturedURL).toContain("cursor=2026-04-05T11%3A00%3A00Z");
  });

  it("maps API failures to InboxClientError", async () => {
    const client = makeInboxClient({
      baseUrl: "https://api.strait.dev",
      token: "token_123",
      fetch: async () =>
        new Response('{"error":"invalid subscriber token"}', {
          status: 401,
          headers: { "Content-Type": "application/json" },
        }),
    });

    const err = await Effect.runPromise(Effect.flip(client.getUnreadCount()));

    expect(err).toBeInstanceOf(InboxClientError);
    expect(err.status).toBe(401);
    expect(err.path).toBe("/v1/inbox/unread-count");
    expect(err.method).toBe("GET");
    expect(err.details).toBe("invalid subscriber token");
  });

  it("supports async token providers", async () => {
    let capturedAuth = "";

    const client = makeInboxClient({
      baseUrl: "https://api.strait.dev/",
      token: async () => "async_token",
      fetch: (_input, init) => {
        capturedAuth = new Headers(init?.headers).get("Authorization") ?? "";
        return Promise.resolve(new Response('{"count":0}', { status: 200 }));
      },
    });

    const out = await Effect.runPromise(client.markAllRead());

    expect(capturedAuth).toBe("Bearer async_token");
    expect(out.count).toBe(0);
  });

  it("maps response schema drift to InboxClientError", async () => {
    const client = makeInboxClient({
      baseUrl: "https://api.strait.dev",
      token: "token_123",
      fetch: () =>
        Promise.resolve(new Response('{"count":"oops"}', { status: 200 })),
    });

    const err = await Effect.runPromise(Effect.flip(client.getUnreadCount()));

    expect(err).toBeInstanceOf(InboxClientError);
    expect(err.path).toBe("/v1/inbox/unread-count");
    expect(err.details).toContain("response validation failed");
  });

  it("streams inbox feed events and parses JSON payloads", async () => {
    const encoder = new TextEncoder();
    const stream = new ReadableStream<Uint8Array>({
      start(controller) {
        controller.enqueue(
          encoder.encode('event: unread_count\ndata: {"count":2}\n\n')
        );
        controller.enqueue(
          encoder.encode('event: item_updated\ndata: {"id":"item_1"}\n\n')
        );
        controller.close();
      },
    });

    const events: Array<{ event: string; data: unknown }> = [];
    const client = makeInboxClient({
      baseUrl: "https://api.strait.dev",
      token: "token_123",
      fetch: () =>
        Promise.resolve(
          new Response(stream, {
            status: 200,
            headers: { "Content-Type": "text/event-stream" },
          })
        ),
    });

    const connection = await Effect.runPromise(
      client.connectFeed({
        reconnect: { maxAttempts: 1 },
        onEvent: (event) => {
          events.push(event);
        },
      })
    );

    await connection.closed;

    expect(events).toHaveLength(2);
    expect(events[0]).toEqual({ event: "unread_count", data: { count: 2 } });
    expect(events[1]).toEqual({
      event: "item_updated",
      data: { id: "item_1" },
    });
  });

  it("reconnects inbox feed after disconnect", async () => {
    const encoder = new TextEncoder();
    let fetchCalls = 0;
    let secondStreamCancelled = false;

    const firstStream = new ReadableStream<Uint8Array>({
      start(controller) {
        controller.enqueue(encoder.encode('event: tick\ndata: {"n":1}\n\n'));
        controller.close();
      },
    });

    const secondStream = new ReadableStream<Uint8Array>({
      cancel() {
        secondStreamCancelled = true;
      },
      start(controller) {
        controller.enqueue(encoder.encode('event: tick\ndata: {"n":2}\n\n'));
      },
    });

    const events: Array<{ event: string; data: unknown }> = [];
    const reconnectAttempts: number[] = [];

    const client = makeInboxClient({
      baseUrl: "https://api.strait.dev",
      token: "token_123",
      fetch: () => {
        fetchCalls += 1;
        return Promise.resolve(
          new Response(fetchCalls === 1 ? firstStream : secondStream, {
            status: 200,
            headers: { "Content-Type": "text/event-stream" },
          })
        );
      },
    });

    const connection = await Effect.runPromise(
      client.connectFeed({
        reconnect: { baseDelayMs: 5, maxDelayMs: 5, maxAttempts: 3 },
        onEvent: (event) => {
          events.push(event);
        },
        onReconnect: (attempt) => {
          reconnectAttempts.push(attempt);
        },
      })
    );

    await waitFor(() => events.length >= 2, 500);
    connection.close();
    await connection.closed;

    expect(fetchCalls).toBe(2);
    expect(events[0]).toEqual({ event: "tick", data: { n: 1 } });
    expect(events[1]).toEqual({ event: "tick", data: { n: 2 } });
    expect(reconnectAttempts).toEqual([1]);
    expect(secondStreamCancelled).toBe(true);
  });

  it("reconnects inbox feed after heartbeat timeout", async () => {
    const encoder = new TextEncoder();
    let fetchCalls = 0;
    let firstStreamCancelled = false;

    const firstStream = new ReadableStream<Uint8Array>({
      cancel() {
        firstStreamCancelled = true;
      },
      start() {
        // Keep stream open without heartbeats.
      },
    });

    const secondStream = new ReadableStream<Uint8Array>({
      start(controller) {
        controller.enqueue(
          encoder.encode('event: heartbeat\ndata: {"ok":true}\n\n')
        );
      },
    });

    const events: Array<{ event: string; data: unknown }> = [];
    const reconnectAttempts: number[] = [];

    const client = makeInboxClient({
      baseUrl: "https://api.strait.dev",
      token: "token_123",
      fetch: () => {
        fetchCalls += 1;
        return Promise.resolve(
          new Response(fetchCalls === 1 ? firstStream : secondStream, {
            status: 200,
            headers: { "Content-Type": "text/event-stream" },
          })
        );
      },
    });

    const connection = await Effect.runPromise(
      client.connectFeed({
        heartbeatTimeoutMs: 20,
        reconnect: { baseDelayMs: 5, maxDelayMs: 5, maxAttempts: 3 },
        onEvent: (event) => {
          events.push(event);
        },
        onReconnect: (attempt) => {
          reconnectAttempts.push(attempt);
        },
      })
    );

    await waitFor(() => events.length >= 1, 800);
    connection.close();
    await connection.closed;

    expect(fetchCalls).toBe(2);
    expect(firstStreamCancelled).toBe(true);
    expect(reconnectAttempts).toEqual([1]);
    expect(events[0]).toEqual({ event: "heartbeat", data: { ok: true } });
  });

  it("allows inbox feed connections to close cleanly", async () => {
    let cancelled = false;
    let closeEvents = 0;

    const stream = new ReadableStream<Uint8Array>({
      cancel() {
        cancelled = true;
      },
      start() {
        // Keep stream open until connection.close() is called.
      },
    });

    const client = makeInboxClient({
      baseUrl: "https://api.strait.dev",
      token: "token_123",
      fetch: () =>
        Promise.resolve(
          new Response(stream, {
            status: 200,
            headers: { "Content-Type": "text/event-stream" },
          })
        ),
    });

    const connection = await Effect.runPromise(
      client.connectFeed({
        onClose: () => {
          closeEvents += 1;
        },
      })
    );

    connection.close();
    await connection.closed;

    expect(cancelled).toBe(true);
    expect(closeEvents).toBe(1);
  });
});

const waitFor = async (
  predicate: () => boolean,
  timeoutMs: number
): Promise<void> => {
  const start = Date.now();
  while (!predicate()) {
    if (Date.now() - start > timeoutMs) {
      throw new Error(`timed out waiting for condition after ${timeoutMs}ms`);
    }
    await new Promise((resolve) => {
      setTimeout(resolve, 5);
    });
  }
};
