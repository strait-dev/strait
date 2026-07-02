import fs from "node:fs";
import http from "node:http";

const port = Number(process.env.E2E_FAKE_ENDPOINT_PORT || "0");
const infoPath =
  process.env.E2E_FAKE_ENDPOINT_INFO_PATH ||
  "playwright/.auth/fake-endpoint.json";
const publicHost = process.env.E2E_FAKE_ENDPOINT_PUBLIC_HOST || "127.0.0.1";
const retryAttempts = new Map();
const requests = [];

const server = http.createServer(async (req, res) => {
  const url = new URL(req.url || "/", "http://127.0.0.1");
  const body = await readBody(req);

  if (req.method === "GET") {
    return handleGet(res, url);
  }
  if (req.method === "DELETE") {
    return handleDelete(res, url);
  }
  if (req.method === "POST") {
    recordRequest(req, url, body);
    return handlePost(req, res, url, body);
  }
  return json(res, 405, { error: "method not allowed" });
});

server.listen(port, "127.0.0.1", () => {
  const address = server.address();
  const actualPort =
    typeof address === "object" && address ? address.port : port;
  const url = `http://${publicHost}:${actualPort}`;
  fs.mkdirSync(infoPath.split("/").slice(0, -1).join("/"), { recursive: true });
  fs.writeFileSync(
    infoPath,
    JSON.stringify({ url, pid: process.pid, managed: true })
  );
  process.stdout.write(`fake endpoint listening at ${url}\n`);
});

for (const signal of ["SIGINT", "SIGTERM"]) {
  process.on(signal, () => {
    server.close(() => process.exit(0));
  });
}

function json(res, status, payload) {
  res.writeHead(status, { "Content-Type": "application/json" });
  res.end(JSON.stringify(payload));
}

function readBody(req) {
  return new Promise((resolve, reject) => {
    const chunks = [];
    req.on("data", (chunk) => chunks.push(chunk));
    req.on("end", () => resolve(Buffer.concat(chunks).toString("utf-8")));
    req.on("error", reject);
  });
}

function parseBody(body) {
  if (!body) {
    return null;
  }

  try {
    return JSON.parse(body);
  } catch {
    return body;
  }
}

function handleGet(res, url) {
  if (url.pathname === "/health") {
    return json(res, 200, { ok: true });
  }
  if (url.pathname === "/requests") {
    const name = url.searchParams.get("name");
    const data = name
      ? requests.filter((entry) => entry.query.name === name)
      : requests;
    return json(res, 200, { data });
  }
  return json(res, 404, { error: "not found" });
}

function handleDelete(res, url) {
  if (url.pathname !== "/requests") {
    return json(res, 404, { error: "not found" });
  }
  requests.length = 0;
  retryAttempts.clear();
  return json(res, 200, { ok: true });
}

async function handlePost(req, res, url, body) {
  if (url.pathname === "/success" || url.pathname === "/echo") {
    return json(res, 200, {
      ok: true,
      path: url.pathname,
      method: req.method,
      body: parseBody(body),
    });
  }
  if (url.pathname === "/fail") {
    return json(res, 500, { error: "fake endpoint failure" });
  }
  if (url.pathname === "/timeout") {
    const delayMs = Number(url.searchParams.get("delay_ms") || "10000");
    await new Promise((resolve) => setTimeout(resolve, delayMs));
    return json(res, 200, { ok: true, delayed_ms: delayMs });
  }
  if (url.pathname === "/retry-then-success") {
    return handleRetryThenSuccess(res, url);
  }
  if (url.pathname.startsWith("/status/")) {
    const status = Number(url.pathname.split("/").at(-1));
    return json(res, Number.isInteger(status) ? status : 500, {
      status,
      body: parseBody(body),
    });
  }
  return json(res, 404, { error: "not found" });
}

function handleRetryThenSuccess(res, url) {
  const key = url.searchParams.get("key") || "default";
  const failures = Number(url.searchParams.get("failures") || "1");
  const attempt = (retryAttempts.get(key) || 0) + 1;
  retryAttempts.set(key, attempt);

  if (attempt <= failures) {
    return json(res, 500, { error: "planned failure", attempt });
  }

  return json(res, 200, { ok: true, attempt });
}

function recordRequest(req, url, body) {
  requests.push({
    id: `${Date.now()}-${requests.length}`,
    method: req.method,
    path: url.pathname,
    query: Object.fromEntries(url.searchParams.entries()),
    headers: req.headers,
    body: parseBody(body),
    received_at: new Date().toISOString(),
  });
  if (requests.length > 500) {
    requests.splice(0, requests.length - 500);
  }
}
