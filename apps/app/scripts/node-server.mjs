/**
 * Node.js HTTP adapter for the TanStack Start fetch handler.
 *
 * Vite builds `dist/server/server.js` as a Web-Fetch-API module
 * (`export default { fetch(request): Response }`) — the same shape a
 * Cloudflare Worker has. Node cannot run a Worker directly, so this
 * script wraps the handler in `node:http` and listens on PORT.
 *
 * Used only by the self-host Docker image (apps/app/Dockerfile). The
 * Cloudflare Workers production deploy at strait.dev does not touch
 * this file.
 */
import { createServer } from "node:http";
import { Readable } from "node:stream";
import handlerModule from "../dist/server/server.js";

const handler = handlerModule.default ?? handlerModule;
const port = Number(process.env.PORT) || 3000;
const host = process.env.HOST || "0.0.0.0";

function toWebRequest(req) {
  const protocol = req.headers["x-forwarded-proto"] || "http";
  const host = req.headers.host || `localhost:${port}`;
  const url = new URL(req.url, `${protocol}://${host}`).toString();

  const headers = new Headers();
  for (const [key, value] of Object.entries(req.headers)) {
    if (Array.isArray(value)) {
      for (const v of value) headers.append(key, v);
    } else if (value !== undefined) {
      headers.set(key, value);
    }
  }

  const method = req.method || "GET";
  const body =
    method === "GET" || method === "HEAD" ? undefined : Readable.toWeb(req);

  return new Request(url, {
    method,
    headers,
    body,
    duplex: "half",
  });
}

async function writeWebResponse(res, webResponse) {
  res.statusCode = webResponse.status;
  res.statusMessage = webResponse.statusText;
  webResponse.headers.forEach((value, key) => {
    res.setHeader(key, value);
  });

  if (!webResponse.body) {
    res.end();
    return;
  }

  const nodeStream = Readable.fromWeb(webResponse.body);
  nodeStream.pipe(res);
  nodeStream.on("error", (err) => {
    console.error("response stream error:", err);
    res.destroy(err);
  });
}

const server = createServer(async (req, res) => {
  try {
    const webRequest = toWebRequest(req);
    const webResponse = await handler.fetch(webRequest);
    await writeWebResponse(res, webResponse);
  } catch (err) {
    console.error("request handler error:", err);
    if (!res.headersSent) {
      res.statusCode = 500;
      res.setHeader("content-type", "text/plain");
    }
    res.end("Internal Server Error");
  }
});

server.listen(port, host, () => {
  console.log(
    JSON.stringify({
      msg: "strait-app listening",
      host,
      port,
    })
  );
});

const shutdown = (signal) => {
  console.log(JSON.stringify({ msg: "shutdown", signal }));
  server.close(() => process.exit(0));
  setTimeout(() => process.exit(1), 10_000).unref();
};
process.on("SIGTERM", () => shutdown("SIGTERM"));
process.on("SIGINT", () => shutdown("SIGINT"));
