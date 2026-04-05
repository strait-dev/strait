# @strait/inbox

Effect-first client for Strait Notify subscriber endpoints with runtime response validation via Effect Schema.

## Scope

This package wraps the subscriber-facing Notify API surface:

- `/v1/inbox`
- `/v1/inbox/unread-count`
- `/v1/inbox/{itemID}`
- `/v1/inbox/{itemID}/action`
- `/v1/inbox/mark-all-read`
- `/v1/inbox/feed`
- `/v1/preferences`
- `/v1/preferences/{scope}`
- `/v1/unsubscribe/{token}`
- `/v1/unsubscribe/{token}/one-click`

## Usage

```ts
import { Effect } from "effect";
import { makeInboxClient } from "@strait/inbox";

const client = makeInboxClient({
  baseUrl: "https://api.strait.dev",
  token: () => process.env.NOTIFY_SUBSCRIBER_TOKEN ?? "",
});

const items = await Effect.runPromise(client.listInbox({ limit: 20 }));

const feed = await Effect.runPromise(
  client.connectFeed({
    onEvent: (event) => {
      if (event.event === "unread_count") {
        console.log("unread count update", event.data);
      }
    },
  })
);

// Later:
feed.close();
await feed.closed;
```

## Error handling

Every operation returns `Effect.Effect<Success, InboxClientError>`.

`InboxClientError` includes:

- `path`
- `method`
- `status` (when available)
- `details`
- `cause` (for transport/runtime failures)
