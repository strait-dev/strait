import { createHash, timingSafeEqual } from "node:crypto";
import {
  resolveTransactionalEmailTemplate,
  type TransactionalEmailTemplateId,
} from "@strait/transactional/registry";
import { z } from "zod";
import { getResend } from "@/lib/resend.server";

const INTERNAL_SECRET_HEADER = "X-Internal-Secret";

const attachmentSchema = z.object({
  filename: z.string().min(1),
  contentBase64: z.string().min(1),
  contentType: z.string().min(1).optional(),
});

const requestSchema = z.object({
  template: z.string().min(1),
  to: z.array(z.email()).min(1),
  from: z.string().min(1).optional(),
  idempotencyKey: z.string().min(1),
  props: z.record(z.string(), z.unknown()),
  attachments: z.array(attachmentSchema).optional(),
});

const json = (body: unknown, status = 200): Response =>
  Response.json(body, { status });

const digest = (value: string): Buffer =>
  createHash("sha256").update(value).digest();

function hasValidInternalSecret(request: Request): boolean {
  const expected = process.env.INTERNAL_SECRET;
  const provided = request.headers.get(INTERNAL_SECRET_HEADER);
  if (!(expected && provided)) {
    return false;
  }
  return timingSafeEqual(digest(expected), digest(provided));
}

const defaultFromEmail = (): string =>
  process.env.RESEND_FROM_EMAIL ??
  process.env.RESEND_SUPPORT_EMAIL ??
  "noreply@strait.dev";

export async function handleTransactionalEmailRequest(
  request: Request
): Promise<Response> {
  if (!process.env.INTERNAL_SECRET) {
    return json({ error: "INTERNAL_SECRET is not configured" }, 500);
  }
  if (!hasValidInternalSecret(request)) {
    return json({ error: "Unauthorized" }, 401);
  }

  let body: unknown;
  try {
    body = await request.json();
  } catch {
    return json({ error: "Invalid JSON body" }, 400);
  }

  const parsed = requestSchema.safeParse(body);
  if (!parsed.success) {
    return json({ error: "Invalid transactional email request" }, 400);
  }

  const template = resolveTransactionalEmailTemplate(parsed.data.template);
  if (!template) {
    return json({ error: "Unknown transactional email template" }, 400);
  }

  const response = await getResend().emails.send(
    {
      from: parsed.data.from ?? defaultFromEmail(),
      to: parsed.data.to,
      subject: template.subject(parsed.data.props),
      react: template.render(parsed.data.props),
      attachments: parsed.data.attachments?.map((attachment) => ({
        filename: attachment.filename,
        content: attachment.contentBase64,
        contentType: attachment.contentType,
      })),
    },
    {
      idempotencyKey: parsed.data.idempotencyKey,
    }
  );

  if (response.error) {
    return json({ error: response.error.message }, 502);
  }

  return json({ id: response.data?.id ?? null });
}

export type TransactionalEmailRequestBody = z.infer<typeof requestSchema> & {
  template: TransactionalEmailTemplateId;
};
