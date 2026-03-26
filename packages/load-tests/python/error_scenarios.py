"""Simulates various failure modes based on ERROR_SCENARIO env var.

Supports 12 failure scenarios for testing error detection, classification,
retry behavior, and recovery. Each scenario exercises a different failure path
in the run lifecycle.

Scenarios:
  clean_exit           - Happy path baseline (exit 0)
  exit_code_1          - Application error (exit 1)
  exit_code_137        - Simulated OOM kill (SIGKILL)
  oom                  - Real OOM: allocate until killed
  segfault             - NULL pointer dereference crash
  infinite_loop        - Never completes (tests timeout enforcement)
  slow_death           - Runs 5 min then crashes (delayed failure)
  panic_after_checkpoint - Checkpoint then crash (tests recovery)
  sdk_timeout          - Late SDK output (tests endpoint timeouts)
  fork_bomb            - Spawns 100 processes (tests process isolation)
  disk_fill            - Writes to /tmp until full (tests disk limits)
  network_abuse        - 1000 outbound requests (tests network limits)
"""

import os
import signal
import sys
import time

STRAIT_API = os.environ.get("STRAIT_SDK_URL", "")
RUN_ID = os.environ.get("STRAIT_RUN_ID", "")
TOKEN = os.environ.get("STRAIT_RUN_TOKEN", "")

scenario = os.environ.get("ERROR_SCENARIO", "clean_exit")


def sdk_headers():
    return {"Authorization": f"Bearer {TOKEN}", "Content-Type": "application/json"}


def sdk_url(path):
    return f"{STRAIT_API}/sdk/v1/runs/{RUN_ID}/{path}"


if scenario == "segfault":
    import ctypes

    ctypes.string_at(0)

elif scenario == "oom":
    data = []
    while True:
        data.append(b"x" * (100 * 1024 * 1024))  # 100MB chunks

elif scenario == "infinite_loop":
    while True:
        time.sleep(0.1)

elif scenario == "exit_code_1":
    print("Application error: exit code 1", file=sys.stderr)
    sys.exit(1)

elif scenario == "exit_code_137":
    os.kill(os.getpid(), signal.SIGKILL)

elif scenario == "slow_death":
    time.sleep(300)
    raise RuntimeError("Delayed failure after 5 minutes")

elif scenario == "panic_after_checkpoint":
    import requests

    requests.post(
        sdk_url("checkpoint"),
        headers=sdk_headers(),
        json={"state": {"progress": 0.5, "data": "important"}},
        timeout=10,
    )
    time.sleep(1)
    raise RuntimeError("Crash after checkpoint -- should be recoverable")

elif scenario == "sdk_timeout":
    import requests

    time.sleep(60)
    requests.post(
        sdk_url("output"),
        headers=sdk_headers(),
        json={"output": {"late": True}},
        timeout=10,
    )

elif scenario == "fork_bomb":
    import subprocess

    for _ in range(100):
        subprocess.Popen([sys.executable, "-c", "import time; time.sleep(60)"])
    time.sleep(30)

elif scenario == "disk_fill":
    with open("/tmp/bigfile", "wb") as f:
        while True:
            f.write(b"x" * (1024 * 1024))

elif scenario == "network_abuse":
    import requests as req

    for i in range(1000):
        try:
            req.get("https://httpbin.org/get", timeout=1)
        except Exception:
            pass

elif scenario == "clean_exit":
    import requests

    if STRAIT_API and RUN_ID and TOKEN:
        requests.post(
            sdk_url("output"),
            headers=sdk_headers(),
            json={"output": {"scenario": "clean_exit", "status": "ok"}},
            timeout=10,
        )
        requests.post(
            sdk_url("complete"),
            headers=sdk_headers(),
            json={},
            timeout=10,
        )
    print("Clean exit -- no errors")
    sys.exit(0)

else:
    print(f"Unknown scenario: {scenario}", file=sys.stderr)
    sys.exit(2)
