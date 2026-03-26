"""CPU-intensive work for 30-120 seconds. Heartbeats every 5s. Checkpoints every 30s.

Computes prime numbers as real CPU work. Used to test long-running job handling,
heartbeat monitoring, checkpoint recovery, and progress reporting.
"""

import json
import math
import os
import sys
import threading
import time

import requests

STRAIT_API = os.environ["STRAIT_SDK_URL"]
RUN_ID = os.environ["STRAIT_RUN_ID"]
TOKEN = os.environ["STRAIT_RUN_TOKEN"]
HEADERS = {"Authorization": f"Bearer {TOKEN}", "Content-Type": "application/json"}

BASE = f"{STRAIT_API}/sdk/v1/runs/{RUN_ID}"

done = False


def heartbeat_loop():
    """Background thread: sends heartbeat every 5 seconds."""
    while not done:
        try:
            requests.post(f"{BASE}/heartbeat", headers=HEADERS, timeout=5)
        except Exception:
            pass
        time.sleep(5)


def main():
    global done

    duration = int(os.environ.get("WORK_DURATION", "60"))

    # Start background heartbeat
    hb = threading.Thread(target=heartbeat_loop, daemon=True)
    hb.start()

    start = time.time()
    iteration = 0
    last_checkpoint = start
    last_progress = start

    while time.time() - start < duration:
        # Real CPU work: primality test on large numbers
        n = 100000 + iteration
        _ = all(n % i != 0 for i in range(2, int(math.sqrt(n)) + 1))
        iteration += 1

        now = time.time()
        elapsed = now - start

        # Checkpoint every 30 seconds
        if now - last_checkpoint >= 30:
            try:
                requests.post(
                    f"{BASE}/checkpoint",
                    headers=HEADERS,
                    json={
                        "state": {
                            "iteration": iteration,
                            "elapsed": elapsed,
                        }
                    },
                    timeout=10,
                )
            except Exception:
                pass
            last_checkpoint = now

        # Progress every 10 seconds
        if now - last_progress >= 10:
            progress = min(1.0, elapsed / duration)
            try:
                requests.post(
                    f"{BASE}/progress",
                    headers=HEADERS,
                    json={
                        "progress": progress,
                        "message": f"Iteration {iteration}, {elapsed:.0f}s elapsed",
                    },
                    timeout=10,
                )
            except Exception:
                pass
            last_progress = now

    done = True

    # Report output and complete
    result = {
        "iterations": iteration,
        "duration": time.time() - start,
        "iterations_per_second": iteration / max(time.time() - start, 0.001),
    }
    requests.post(
        f"{BASE}/output",
        headers=HEADERS,
        json={"output": result},
        timeout=10,
    )
    requests.post(f"{BASE}/complete", headers=HEADERS, json={}, timeout=10)


if __name__ == "__main__":
    try:
        main()
    except Exception as e:
        done = True
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
