import type { EventTrigger, SendEventOptions, WaitForEventOptions } from "./types";

/** Event trigger client for the Strait SDK. */
export class EventsClient {
  private baseUrl: string;
  private headers: Record<string, string>;

  constructor(baseUrl: string, headers: Record<string, string>) {
    this.baseUrl = baseUrl.replace(/\/$/, "");
    this.headers = headers;
  }

  /**
   * Pause the current run and wait for an external event.
   * The run transitions to "waiting" status and is re-dispatched
   * when the event arrives with the event payload as checkpoint data.
   */
  async waitForEvent(runId: string, options: WaitForEventOptions): Promise<EventTrigger> {
    const url = `${this.baseUrl}/sdk/v1/runs/${runId}/wait-for-event`;
    const body = {
      event_key: options.eventKey,
      timeout_secs: options.timeoutSecs ?? 3600,
      notify_url: options.notifyUrl,
    };

    const resp = await fetch(url, {
      method: "POST",
      headers: { ...this.headers, "Content-Type": "application/json" },
      body: JSON.stringify(body),
    });

    if (!resp.ok) {
      const err = await resp.json().catch(() => ({ error: resp.statusText }));
      throw new Error(`waitForEvent failed (${resp.status}): ${err.error || resp.statusText}`);
    }

    return resp.json() as Promise<EventTrigger>;
  }

  /**
   * Send an event to resolve a waiting trigger.
   * Returns the updated trigger with status "received".
   */
  async sendEvent(options: SendEventOptions): Promise<EventTrigger> {
    const url = `${this.baseUrl}/v1/events/${encodeURIComponent(options.eventKey)}/send`;
    const body = options.payload ? { payload: options.payload } : {};

    const resp = await fetch(url, {
      method: "POST",
      headers: { ...this.headers, "Content-Type": "application/json" },
      body: JSON.stringify(body),
    });

    if (!resp.ok) {
      const err = await resp.json().catch(() => ({ error: resp.statusText }));
      throw new Error(`sendEvent failed (${resp.status}): ${err.error || resp.statusText}`);
    }

    return resp.json() as Promise<EventTrigger>;
  }

  /** Get an event trigger by its event key. */
  async getEventTrigger(eventKey: string): Promise<EventTrigger> {
    const url = `${this.baseUrl}/v1/events/${encodeURIComponent(eventKey)}`;

    const resp = await fetch(url, {
      method: "GET",
      headers: this.headers,
    });

    if (!resp.ok) {
      const err = await resp.json().catch(() => ({ error: resp.statusText }));
      throw new Error(`getEventTrigger failed (${resp.status}): ${err.error || resp.statusText}`);
    }

    return resp.json() as Promise<EventTrigger>;
  }

  /** Cancel a waiting event trigger. */
  async cancelEventTrigger(eventKey: string): Promise<EventTrigger> {
    const url = `${this.baseUrl}/v1/events/${encodeURIComponent(eventKey)}`;

    const resp = await fetch(url, {
      method: "DELETE",
      headers: this.headers,
    });

    if (!resp.ok) {
      const err = await resp.json().catch(() => ({ error: resp.statusText }));
      throw new Error(`cancelEventTrigger failed (${resp.status}): ${err.error || resp.statusText}`);
    }

    return resp.json() as Promise<EventTrigger>;
  }
}
