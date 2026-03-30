import { useCallback, useEffect, useRef, useState } from "react";

type StreamState = {
  chunks: string[];
  connected: boolean;
  error: string | null;
};

export function useAgentStream(
  runId: string,
  enabled: boolean,
  baseUrl = ""
): StreamState {
  const [state, setState] = useState<StreamState>({
    chunks: [],
    connected: false,
    error: null,
  });
  const esRef = useRef<EventSource | null>(null);
  const retriesRef = useRef(0);
  const retryTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const mountedRef = useRef(true);

  const connect = useCallback(() => {
    if (!(enabled && runId && mountedRef.current)) {
      return;
    }

    const url = `${baseUrl}/v1/runs/${runId}/stream/chunks`;
    const es = new EventSource(url);
    esRef.current = es;

    es.onopen = () => {
      retriesRef.current = 0;
      setState((prev) => ({ ...prev, connected: true, error: null }));
    };

    es.onmessage = (event) => {
      if (!event.data) {
        return;
      }
      setState((prev) => ({
        ...prev,
        chunks: [...prev.chunks, event.data],
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
  }, [runId, enabled, baseUrl]);

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
