import { randomUUID } from "node:crypto";
import fs from "node:fs";
import path from "node:path";

export type LocalEmail = {
  id: string;
  createdAt: string;
  from?: string;
  to: string | string[];
  subject?: string;
  html?: string;
};

const DEFAULT_OUTBOX_PATH = path.resolve(
  process.cwd(),
  "playwright/.auth/local-emails.jsonl"
);

export function shouldUseLocalEmailOutbox() {
  const apiKey = process.env.RESEND_API_KEY;
  return (
    process.env.LOCAL_EMAIL_OUTBOX === "1" ||
    !apiKey ||
    apiKey === "re_dogfood_local_dummy" ||
    apiKey.startsWith("re_e2e_")
  );
}

function localEmailOutboxPath() {
  return process.env.LOCAL_EMAIL_OUTBOX_PATH || DEFAULT_OUTBOX_PATH;
}

export function appendLocalEmail(
  message: Omit<LocalEmail, "id" | "createdAt">
) {
  const email = {
    id: randomUUID(),
    createdAt: new Date().toISOString(),
    ...message,
  };
  const outboxPath = localEmailOutboxPath();
  fs.mkdirSync(path.dirname(outboxPath), { recursive: true });
  fs.appendFileSync(outboxPath, `${JSON.stringify(email)}\n`, "utf-8");
  return email;
}
