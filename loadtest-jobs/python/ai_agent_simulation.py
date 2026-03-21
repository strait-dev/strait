"""Simulates an AI agent: multiple LLM calls, tool calls, streaming, cost tracking.

Runs 3-10 iterations of a think-tool-respond loop. Each iteration:
- Simulates LLM inference delay (1-3s)
- Reports LLM token usage
- Records a tool call
- Streams output chunks
- Creates a checkpoint

Used to test SDK telemetry endpoints under load with realistic agent patterns.
"""

import json
import os
import sys
import time

import requests

STRAIT_API = os.environ["STRAIT_SDK_URL"]
RUN_ID = os.environ["STRAIT_RUN_ID"]
TOKEN = os.environ["STRAIT_RUN_TOKEN"]
HEADERS = {"Authorization": f"Bearer {TOKEN}", "Content-Type": "application/json"}

BASE = f"{STRAIT_API}/sdk/v1/runs/{RUN_ID}"


def main():
    iterations = int(os.environ.get("AGENT_ITERATIONS", "5"))

    # Send initial heartbeat
    requests.post(f"{BASE}/heartbeat", headers=HEADERS, timeout=5)

    for i in range(iterations):
        # Simulate LLM inference latency (1-3 seconds like a real API call)
        time.sleep(1 + (i % 3))

        # Report LLM usage with escalating token counts (realistic agent pattern)
        requests.post(
            f"{BASE}/usage",
            headers=HEADERS,
            json={
                "provider": "anthropic",
                "model": "claude-sonnet-4-20250514",
                "prompt_tokens": 800 + i * 200,
                "completion_tokens": 300 + i * 50,
            },
            timeout=10,
        )

        # Record tool call
        requests.post(
            f"{BASE}/tool-call",
            headers=HEADERS,
            json={
                "tool_name": f"search_{i}",
                "input": {"query": f"iteration {i}"},
                "output": {"results": [f"result_{j}" for j in range(3)]},
                "duration_ms": 500 + i * 100,
                "status": "success",
            },
            timeout=10,
        )

        # Stream some output
        requests.post(
            f"{BASE}/stream",
            headers=HEADERS,
            json={"chunk": f"Agent thinking step {i + 1}/{iterations}...\n"},
            timeout=10,
        )

        # Checkpoint after each iteration
        requests.post(
            f"{BASE}/checkpoint",
            headers=HEADERS,
            json={
                "state": {
                    "iteration": i,
                    "tools_called": i + 1,
                }
            },
            timeout=10,
        )

        # Progress
        requests.post(
            f"{BASE}/progress",
            headers=HEADERS,
            json={
                "progress": (i + 1) / iterations,
                "message": f"Completed agent iteration {i + 1}/{iterations}",
            },
            timeout=10,
        )

        # Heartbeat
        requests.post(f"{BASE}/heartbeat", headers=HEADERS, timeout=5)

    # Final output and completion
    requests.post(
        f"{BASE}/output",
        headers=HEADERS,
        json={
            "output": {
                "iterations": iterations,
                "total_tool_calls": iterations,
                "total_prompt_tokens": sum(800 + i * 200 for i in range(iterations)),
                "total_completion_tokens": sum(300 + i * 50 for i in range(iterations)),
            }
        },
        timeout=10,
    )
    requests.post(f"{BASE}/complete", headers=HEADERS, json={}, timeout=10)


if __name__ == "__main__":
    try:
        main()
    except Exception as e:
        try:
            requests.post(
                f"{BASE}/fail",
                headers=HEADERS,
                json={"error": str(e)},
                timeout=5,
            )
        except Exception:
            pass
        print(f"FATAL: {e}", file=sys.stderr)
        sys.exit(1)
