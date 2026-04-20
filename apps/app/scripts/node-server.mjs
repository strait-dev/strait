/**
 * Node.js HTTP adapter for the TanStack Start fetch handler.
 *
 * Vite builds `dist/server/server.js` as a Web-Fetch-API module
 * (`export default { fetch(request): Response }`) — the same shape a
 * Cloudflare Worker has. Node cannot run a Worker directly, so this
 * script wraps the handler in `node:http` and listens on PORT.
 *
 * It also serves static assets out of `dist/client/`, which in the
 * Cloudflare deploy are served by Workers Assets automatically but
 * are the Node runtime's responsibility here.
 *
 * Used only by the self-host Docker image (apps/app/Dockerfile). The
 * Cloudflare Workers production deploy at strait.dev does not touch
 * this file.
 */

import { createReadStream } from "node:fs";
import { stat } from "node:fs/promises";
import { createServer } from "node:http";
import path from "node:path";
import { Readable } from "node:stream";
import { fileURLToPath } from "node:url";
import handlerModule from "../dist/server/server.js";

const handler = handlerModule.default ?? handlerModule;
const port = Number(process.env.PORT) || 3000;
const host = process.env.HOST || "0.0.0.0";

const scriptDir = path.dirname(fileURLToPath(import.meta.url));
const leadingSlashesRe = /^\/+/;
const clientDir = path.resolve(scriptDir, "..", "dist", "client");

const mimeTypes = new Map([
  [".html", "text/html; charset=utf-8"],
  [".js", "application/javascript; charset=utf-8"],
  [".mjs", "application/javascript; charset=utf-8"],
  [".css", "text/css; charset=utf-8"],
  [".json", "application/json; charset=utf-8"],
  [".txt", "text/plain; charset=utf-8"],
  [".svg", "image/svg+xml"],
  [".png", "image/png"],
  [".jpg", "image/jpeg"],
  [".jpeg", "image/jpeg"],
  [".gif", "image/gif"],
  [".webp", "image/webp"],
  [".ico", "image/x-icon"],
  [".woff", "font/woff"],
  [".woff2", "font/woff2"],
  [".ttf", "font/ttf"],
  [".map", "application/json; charset=utf-8"],
  [".wasm", "application/wasm"],
]);

function contentTypeFor(filePath) {
  return mimeTypes.get(path.extname(filePath)) ?? "application/octet-stream";
}

async function resolveStaticFile(urlPath) {
  if (urlPath === "/" || urlPath === "") {
    return null;
  }
  const decoded = decodeURIComponent(urlPath.split("?")[0]);
  const relative = decoded.replace(leadingSlashesRe, "");
  const candidate = path.resolve(clientDir, relative);
  if (
    !candidate.startsWith(`${clientDir}${path.sep}`) &&
    candidate !== clientDir
  ) {
    return null;
  }
  try {
    const stats = await stat(candidate);
    if (!stats.isFile()) {
      return null;
    }
    return { path: candidate, size: stats.size };
  } catch {
    return null;
  }
}

function isImmutableAsset(urlPath) {
  // Vite emits content-hashed files under /assets/. Safe to cache forever.
  return urlPath.startsWith("/assets/");
}

async function tryServeStatic(req, res) {
  if (req.method !== "GET" && req.method !== "HEAD") {
    return false;
  }
  const urlPath = (req.url || "/").split("?")[0];
  const file = await resolveStaticFile(urlPath);
  if (!file) {
    return false;
  }

  res.setHeader("content-type", contentTypeFor(file.path));
  res.setHeader("content-length", file.size);
  res.setHeader(
    "cache-control",
    isImmutableAsset(urlPath)
      ? "public, max-age=31536000, immutable"
      : "public, max-age=300"
  );

  if (req.method === "HEAD") {
    res.statusCode = 200;
    res.end();
    return true;
  }

  res.statusCode = 200;
  const stream = createReadStream(file.path);
  stream.on("error", (err) => {
    console.error("static file stream error:", err);
    if (!res.headersSent) {
      res.statusCode = 500;
    }
    res.destroy(err);
  });
  stream.pipe(res);
  return true;
}

function toWebRequest(req) {
  const protocol = req.headers["x-forwarded-proto"] || "http";
  const reqHost = req.headers.host || `localhost:${port}`;
  const url = new URL(req.url, `${protocol}://${reqHost}`).toString();

  const headers = new Headers();
  for (const [key, value] of Object.entries(req.headers)) {
    if (Array.isArray(value)) {
      for (const v of value) {
        headers.append(key, v);
      }
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

function writeWebResponse(res, webResponse) {
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
    if (await tryServeStatic(req, res)) {
      return;
    }
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
      clientDir,
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
