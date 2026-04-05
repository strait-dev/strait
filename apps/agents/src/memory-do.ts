// Cloudflare Workers runtime types (not available in node typecheck).
// These are provided by the CF Workers runtime at execution time.
/* eslint-disable @typescript-eslint/no-explicit-any */
declare class DurableObject<E = unknown> {
  constructor(ctx: any, env: E);
}
declare interface DurableObjectState {
  storage: {
    sql: SqlStorage;
    setAlarm(scheduledTime: number): void;
  };
}
declare interface SqlStorage {
  exec(query: string, ...params: unknown[]): SqlStorageResult;
}
declare interface SqlStorageResult {
  toArray(): Record<string, unknown>[];
}
/* eslint-enable @typescript-eslint/no-explicit-any */

// biome-ignore lint/suspicious/noControlCharactersInRegex: intentional validation of user input
const CONTROL_CHARS_RE = /[\x00-\x1f]/;

function hasControlChars(value: string): boolean {
  return CONTROL_CHARS_RE.test(value);
}

type MemoryEntry = {
  key: string;
  value: string;
  size_bytes: number;
  ttl_expires_at: number | null;
  created_at: number;
  updated_at: number;
};

/**
 * AgentMemoryDO provides per-agent persistent memory using Durable Object SQLite.
 * Each agent definition maps to one DO instance (ID = agent ID).
 * Provides atomic read-modify-write guarantees and cross-run consistency.
 */
export class AgentMemoryDO extends DurableObject<Env> {
  private readonly sql: SqlStorage;
  private readonly state: DurableObjectState;

  constructor(ctx: DurableObjectState, env: Env) {
    super(ctx, env);
    this.state = ctx;
    this.sql = ctx.storage.sql;
    this.sql.exec(`
      CREATE TABLE IF NOT EXISTS memory (
        key TEXT PRIMARY KEY,
        value TEXT NOT NULL,
        size_bytes INTEGER NOT NULL,
        ttl_expires_at INTEGER,
        created_at INTEGER NOT NULL,
        updated_at INTEGER NOT NULL
      )
    `);

    // Schedule periodic TTL cleanup via alarm.
    ctx.storage.setAlarm(Date.now() + 60_000);
  }

  async fetch(request: Request): Promise<Response> {
    const url = new URL(request.url);
    const parts = url.pathname.split("/").filter(Boolean);

    // Routes:
    // GET    /memory       -> list all keys
    // GET    /memory/:key  -> get single key
    // POST   /memory/:key  -> set key (body: { value, ttl_secs? })
    // DELETE /memory/:key  -> delete key
    // GET    /memory/_size -> total size in bytes

    if (parts[0] !== "memory") {
      return new Response("not found", { status: 404 });
    }

    const key = parts[1];

    // Validate key: max 256 chars, no control characters (code points 0-31).
    if (key && (key.length > 256 || hasControlChars(key))) {
      return new Response(
        JSON.stringify({
          error: "invalid_key",
          message: "Key must be <= 256 chars with no control characters",
        }),
        { status: 400, headers: { "Content-Type": "application/json" } }
      );
    }

    switch (request.method) {
      case "GET": {
        if (!key) {
          return this.listKeys();
        }
        if (key === "_size") {
          return this.totalSize();
        }
        return this.getKey(key);
      }
      case "POST": {
        if (!key) {
          return new Response("key required", { status: 400 });
        }
        // Enforce max request body size (10MB hard limit).
        const contentLength = Number.parseInt(
          request.headers.get("content-length") || "0",
          10
        );
        if (contentLength > 10 * 1024 * 1024) {
          return new Response(
            JSON.stringify({
              error: "payload_too_large",
              message: "Request body exceeds 10MB",
            }),
            { status: 413, headers: { "Content-Type": "application/json" } }
          );
        }
        const body = (await request.json()) as {
          value: unknown;
          ttl_secs?: number;
          max_per_key?: number;
          max_per_agent?: number;
        };
        return this.setKey(
          key,
          body.value,
          body.ttl_secs,
          body.max_per_key,
          body.max_per_agent
        );
      }
      case "DELETE": {
        if (!key) {
          return new Response("key required", { status: 400 });
        }
        return this.deleteKey(key);
      }
      default:
        return new Response("method not allowed", { status: 405 });
    }
  }

  private getKey(key: string): Response {
    this.cleanExpired();
    const rows = this.sql
      .exec(
        `SELECT key, value, size_bytes, ttl_expires_at, created_at, updated_at
         FROM memory WHERE key = ? AND (ttl_expires_at IS NULL OR ttl_expires_at > ?)`,
        key,
        Date.now()
      )
      .toArray();

    if (rows.length === 0) {
      return new Response("not found", { status: 404 });
    }

    const entry = rows[0];
    if (!entry) {
      return new Response("not found", { status: 404 });
    }
    return Response.json({
      key: entry.key,
      value: JSON.parse(String(entry.value)),
      size_bytes: entry.size_bytes,
      ttl_expires_at: entry.ttl_expires_at,
      created_at: entry.created_at,
      updated_at: entry.updated_at,
    });
  }

  private setKey(
    key: string,
    value: unknown,
    ttlSecs?: number,
    maxPerKey?: number,
    maxPerAgent?: number
  ): Response {
    const serialized = JSON.stringify(value);
    const sizeBytes = new TextEncoder().encode(serialized).length;

    // Per-key size limit.
    if (maxPerKey && maxPerKey > 0 && sizeBytes > maxPerKey) {
      return Response.json(
        {
          error: "value_too_large",
          message: `Value size ${sizeBytes} exceeds limit ${maxPerKey}`,
        },
        { status: 400 }
      );
    }

    // Per-agent total size limit.
    if (maxPerAgent && maxPerAgent > 0) {
      const currentTotal = Number(
        this.sql
          .exec(
            `SELECT COALESCE(SUM(size_bytes), 0) as total FROM memory
           WHERE key != ? AND (ttl_expires_at IS NULL OR ttl_expires_at > ?)`,
            key,
            Date.now()
          )
          .toArray()[0]?.total ?? 0
      );
      if (currentTotal + sizeBytes > maxPerAgent) {
        return Response.json(
          {
            error: "memory_quota_exceeded",
            message: `Total memory ${currentTotal + sizeBytes} would exceed limit ${maxPerAgent}`,
          },
          { status: 400 }
        );
      }
    }

    const now = Date.now();
    // Reject negative or zero TTL — would cause immediate expiry (data loss).
    if (ttlSecs !== undefined && ttlSecs !== null && ttlSecs <= 0) {
      return Response.json(
        {
          error: "invalid_ttl",
          message: "ttl_secs must be a positive integer",
        },
        { status: 400 }
      );
    }
    const ttlExpiresAt = ttlSecs ? now + ttlSecs * 1000 : null;

    this.sql.exec(
      `INSERT INTO memory (key, value, size_bytes, ttl_expires_at, created_at, updated_at)
       VALUES (?, ?, ?, ?, ?, ?)
       ON CONFLICT (key) DO UPDATE SET
         value = excluded.value,
         size_bytes = excluded.size_bytes,
         ttl_expires_at = excluded.ttl_expires_at,
         updated_at = excluded.updated_at`,
      key,
      serialized,
      sizeBytes,
      ttlExpiresAt,
      now,
      now
    );

    return Response.json({
      key,
      value,
      size_bytes: sizeBytes,
      ttl_expires_at: ttlExpiresAt,
      created_at: now,
      updated_at: now,
    });
  }

  private deleteKey(key: string): Response {
    this.sql.exec("DELETE FROM memory WHERE key = ?", key);
    return new Response(null, { status: 204 });
  }

  private listKeys(): Response {
    this.cleanExpired();
    const rows = this.sql
      .exec(
        `SELECT key, value, size_bytes, ttl_expires_at, created_at, updated_at
         FROM memory WHERE ttl_expires_at IS NULL OR ttl_expires_at > ?
         ORDER BY key ASC`,
        Date.now()
      )
      .toArray();

    return Response.json(
      (rows as unknown as MemoryEntry[]).map((entry) => ({
        key: entry.key,
        value: JSON.parse(String(entry.value)),
        size_bytes: entry.size_bytes,
        ttl_expires_at: entry.ttl_expires_at,
        created_at: entry.created_at,
        updated_at: entry.updated_at,
      }))
    );
  }

  private totalSize(): Response {
    const result = this.sql
      .exec(
        `SELECT COALESCE(SUM(size_bytes), 0) as total FROM memory
         WHERE ttl_expires_at IS NULL OR ttl_expires_at > ?`,
        Date.now()
      )
      .toArray()[0];
    return Response.json({ total_bytes: Number(result?.total ?? 0) });
  }

  private cleanExpired(): void {
    this.sql.exec(
      "DELETE FROM memory WHERE ttl_expires_at IS NOT NULL AND ttl_expires_at <= ?",
      Date.now()
    );
  }

  alarm(): void {
    this.cleanExpired();
    // Re-schedule next cleanup in 60 seconds.
    this.state.storage.setAlarm(Date.now() + 60_000);
  }
}

export interface Env {
  AGENT_MEMORY: unknown;
  STRAIT_ENV: string;
}
