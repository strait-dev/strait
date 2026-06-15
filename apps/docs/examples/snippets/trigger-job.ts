const baseUrl = process.env.STRAIT_BASE_URL ?? "https://api.strait.dev";
const apiKey = process.env.STRAIT_API_KEY;

if (!apiKey) {
  throw new Error("STRAIT_API_KEY is required");
}

const response = await fetch(`${baseUrl}/v1/jobs/send-welcome-email/trigger`, {
  method: "POST",
  headers: {
    Authorization: `Bearer ${apiKey}`,
    "Content-Type": "application/json",
  },
  body: JSON.stringify({
    payload: {
      user_id: "user_abc123",
      email: "customer@example.com",
    },
  }),
});

if (!response.ok) {
  throw new Error(await response.text());
}

const run = await response.json();
console.log(run.id, run.status);
