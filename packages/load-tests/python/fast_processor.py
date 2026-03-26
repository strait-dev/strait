"""Fast processor: fetches payload, does minimal work, reports usage, completes via SDK.

Completes in <1 second. Used as the baseline high-throughput job in load tests.
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
    # Fetch payload
    resp = requests.get(f"{BASE}/payload", headers=HEADERS, timeout=10)
    resp.raise_for_status()
    payload = resp.json()

    # Minimal real work: process the payload
    items = payload.get("data", [])
    result = {
        "processed": True,
        "items": len(items),
        "timestamp": time.time(),
    }

    # Report AI usage (simulates token tracking)
    requests.post(
        f"{BASE}/usage",
        headers=HEADERS,
        json={
            "provider": "openai",
            "model": "gpt-4o",
            "prompt_tokens": 150,
            "completion_tokens": 50,
        },
        timeout=10,
    )

    # Complete with output
    requests.post(
        f"{BASE}/output",
        headers=HEADERS,
        json={"output": result},
        timeout=10,
    )

    # Mark complete
    requests.post(f"{BASE}/complete", headers=HEADERS, json={}, timeout=10)


if __name__ == "__main__":
    try:
        main()
    except Exception as e:
        # Report failure via SDK if possible
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
