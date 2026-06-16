import json
import os
import urllib.request


base_url = os.environ.get("STRAIT_BASE_URL", "https://api.strait.dev")
api_key = os.environ["STRAIT_API_KEY"]

body = json.dumps(
    {
        "payload": {
            "user_id": "user_abc123",
            "email": "customer@example.com",
        }
    }
).encode()

request = urllib.request.Request(
    f"{base_url}/v1/jobs/send-welcome-email/trigger",
    data=body,
    method="POST",
    headers={
        "Authorization": f"Bearer {api_key}",
        "Content-Type": "application/json",
    },
)

with urllib.request.urlopen(request, timeout=30) as response:
    run = json.loads(response.read().decode())

print(run["id"], run["status"])
