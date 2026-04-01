import { useCallback, useEffect, useRef, useState } from "react";

export type AgentEventType =
  | "checkpoint"
  | "complete"
  | "fail"
  | "state_change"
  | "stream"
  | "tool_call"
  | "usage";

export type AgentStreamEvent = {
  data: Record<string, unknown>;
  raw: string;
  timestamp: string;
  type: AgentEventType | "unknown";
};

type EventStreamState = {
  connected: boolean;
  error: string | null;
  events: AgentStreamEvent[];
};

function parseEvent(eventType: string, data: string): AgentStreamEvent {
  let parsed: Record<string, unknown> = {};
  try {
    parsed = JSON.parse(data) as Record<string, unknown>;
  } catch {
    parsed = { chunk: data };
  }
  const knownTypes: AgentEventType[] = [
    "checkpoint",
    "complete",
    "fail",
    "state_change",
    "stream",
    "tool_call",
    "usage",
  ];
  const type = knownTypes.includes(eventType as AgentEventType)
    ? (eventType as AgentEventType)
    : "unknown";

  return {
    type,
    data: parsed,
    raw: data,
    timestamp:
      typeof parsed.timestamp === "string"
        ? parsed.timestamp
        : new Date().toISOString(),
  };
}

/** Subscribes to typed SSE events for an agent run (tool calls, usage, checkpoints). */
export function useAgentEventStream(
  agentId: string,
  runId: string,
  enabled: boolean,
  baseUrl = ""
): EventStreamState {
  const [state, setState] = useState<EventStreamState>({
    connected: false,
    error: null,
    events: [],
  });
  const esRef = useRef<EventSource | null>(null);
  const retriesRef = useRef(0);
  const retryTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const mountedRef = useRef(true);

  const connect = useCallback(() => {
    if (!(enabled && runId && agentId && mountedRef.current)) {
      return;
    }

    const url = `${baseUrl}/v1/agents/${agentId}/runs/${runId}/events`;
    const es = new EventSource(url);
    esRef.current = es;

    es.onopen = () => {
      retriesRef.current = 0;
      setState((prev) => ({ ...prev, connected: true, error: null }));
    };

    // Listen for typed events.
    const eventTypes: AgentEventType[] = [
      "tool_call",
      "usage",
      "checkpoint",
      "stream",
      "state_change",
      "complete",
      "fail",
    ];
    for (const eventType of eventTypes) {
      es.addEventListener(eventType, (event: MessageEvent) => {
        if (!event.data) {
          return;
        }
        const parsed = parseEvent(eventType, event.data as string);
        setState((prev) => ({
          ...prev,
          events: [...prev.events, parsed],
        }));
      });
    }

    // Fallback for untyped messages (backward compat).
    es.onmessage = (event: MessageEvent) => {
      if (!event.data) {
        return;
      }
      const parsed = parseEvent("stream", event.data as string);
      setState((prev) => ({
        ...prev,
        events: [...prev.events, parsed],
      }));
    };

    es.onerror = () => {
      es.close();
      esRef.current = null;
      setState((prev) => ({ ...prev, connected: false }));

      if (!mountedRef.current) {
        return;
      }

      if (retriesRef.current < 5) {
        const delay = Math.min(1000 * 2 ** retriesRef.current, 16_000);
        retriesRef.current += 1;
        retryTimerRef.current = setTimeout(connect, delay);
      } else {
        setState((prev) => ({
          ...prev,
          error: "stream disconnected after retries",
        }));
      }
    };
  }, [agentId, runId, enabled, baseUrl]);

  useEffect(() => {
    mountedRef.current = true;
    if (enabled) {
      connect();
    }
    return () => {
      mountedRef.current = false;
      if (retryTimerRef.current != null) {
        clearTimeout(retryTimerRef.current);
        retryTimerRef.current = null;
      }
      esRef.current?.close();
      esRef.current = null;
    };
  }, [connect, enabled]);

  return state;
}
