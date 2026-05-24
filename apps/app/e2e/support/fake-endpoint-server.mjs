import fs from "node:fs";
import http from "node:http";

const port = Number(process.env.E2E_FAKE_ENDPOINT_PORT || "0");
const infoPath =
  process.env.E2E_FAKE_ENDPOINT_INFO_PATH ||
  "playwright/.auth/fake-endpoint.json";
const publicHost = process.env.E2E_FAKE_ENDPOINT_PUBLIC_HOST || "127.0.0.1";
const retryAttempts = new Map();

const server = http.createServer(async (req, res) => {
  const url = new URL(req.url || "/", "http://127.0.0.1");
  const body = await readBody(req);

  if (req.method === "GET" && url.pathname === "/health") {
    return json(res, 200, { ok: true });
  }

  if (req.method !== "POST") {
    return json(res, 405, { error: "method not allowed" });
  }

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
    const key = url.searchParams.get("key") || "default";
    const failures = Number(url.searchParams.get("failures") || "1");
    const attempt = (retryAttempts.get(key) || 0) + 1;
    retryAttempts.set(key, attempt);

    if (attempt <= failures) {
      return json(res, 500, { error: "planned failure", attempt });
    }

    return json(res, 200, { ok: true, attempt });
  }

  if (url.pathname.startsWith("/status/")) {
    const status = Number(url.pathname.split("/").at(-1));
    return json(res, Number.isInteger(status) ? status : 500, {
      status,
      body: parseBody(body),
    });
  }

  return json(res, 404, { error: "not found" });
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
