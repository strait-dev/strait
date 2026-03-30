import { useCallback, useRef, useState } from "react";

type DirectRunState = {
  chunks: string[];
  connected: boolean;
  error: string | null;
  runId: string | null;
};

type DirectRunResponse = {
  envelope: Record<string, unknown>;
  run_id: string;
  token: string;
  worker_url: string;
};

async function readNDJSONStream(
  reader: ReadableStreamDefaultReader<Uint8Array>,
  onChunk: (line: string) => void
) {
  const decoder = new TextDecoder();
  let buffer = "";

  for (;;) {
    const { done, value } = await reader.read();
    if (done) {
      break;
    }

    buffer += decoder.decode(value, { stream: true });
    const lines = buffer.split("\n");
    buffer = lines.pop() ?? "";

    for (const line of lines) {
      if (line.trim()) {
        onChunk(line);
      }
    }
  }
}

export function useDirectAgentRun() {
  const [state, setState] = useState<DirectRunState>({
    chunks: [],
    connected: false,
    error: null,
    runId: null,
  });
  const abortRef = useRef<AbortController | null>(null);

  const run = useCallback(async (directRunData: DirectRunResponse) => {
    abortRef.current?.abort();
    const controller = new AbortController();
    abortRef.current = controller;

    setState({
      chunks: [],
      connected: true,
      error: null,
      runId: directRunData.run_id,
    });

    try {
      const response = await fetch(directRunData.worker_url, {
        method: "POST",
        headers: {
          Authorization: `Bearer ${directRunData.token}`,
          "Content-Type": "application/json",
        },
        body: JSON.stringify(directRunData.envelope),
        signal: controller.signal,
      });

      if (!response.ok) {
        throw new Error(`Worker returned ${response.status}`);
      }

      const reader = response.body?.getReader();
      if (!reader) {
        throw new Error("No response body");
      }

      await readNDJSONStream(reader, (line) => {
        setState((prev) => ({
          ...prev,
          chunks: [...prev.chunks, line],
        }));
      });

      setState((prev) => ({ ...prev, connected: false }));
    } catch (err) {
      if (err instanceof DOMException && err.name === "AbortError") {
        return;
      }
      setState((prev) => ({
        ...prev,
        connected: false,
        error: err instanceof Error ? err.message : "unknown error",
      }));
    }
  }, []);

  const cancel = useCallback(() => {
    abortRef.current?.abort();
    setState((prev) => ({ ...prev, connected: false }));
  }, []);

  return { ...state, run, cancel };
}
