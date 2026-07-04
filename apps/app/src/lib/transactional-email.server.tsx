import { createHash, timingSafeEqual } from "node:crypto";
import { resolveTransactionalEmailTemplate } from "@strait/transactional/registry";
import { z } from "zod";
import { getResend } from "@/lib/resend.server";
import { captureException } from "@/lib/sentry";

const INTERNAL_SECRET_HEADER = "X-Internal-Secret";
const MAX_ATTACHMENT_COUNT = 3;
const MAX_ATTACHMENT_BYTES = 10 * 1024 * 1024;
const PDF_CONTENT_TYPE = "application/pdf";
const FILENAME_PATH_SEPARATOR_PATTERN = /[\\/]/;

const normalizeBase64 = (value: string): string => value.replace(/\s/g, "");

const isBase64Char = (char: string): boolean => {
  const code = char.charCodeAt(0);
  return (
    (code >= 65 && code <= 90) ||
    (code >= 97 && code <= 122) ||
    (code >= 48 && code <= 57) ||
    char === "+" ||
    char === "/"
  );
};

const isValidBase64 = (value: string): boolean => {
  if (value.length === 0 || value.length % 4 !== 0) {
    return false;
  }
  let padding = 0;
  for (const char of value) {
    if (char === "=") {
      padding += 1;
      if (padding > 2) {
        return false;
      }
      continue;
    }
    if (padding > 0 || !isBase64Char(char)) {
      return false;
    }
  }
  return true;
};

const decodedByteLength = (value: string): number =>
  Buffer.from(value, "base64").length;

const attachmentSchema = z.object({
  filename: z
    .string()
    .trim()
    .min(1)
    .refine(
      (value) =>
        !(FILENAME_PATH_SEPARATOR_PATTERN.test(value) || value.includes("\0")),
      {
        message: "filename must not contain path separators",
      }
    ),
  contentBase64: z
    .string()
    .min(1)
    .transform(normalizeBase64)
    .refine(isValidBase64, { message: "contentBase64 must be valid base64" })
    .refine((value) => decodedByteLength(value) <= MAX_ATTACHMENT_BYTES, {
      message: "attachment exceeds maximum decoded size",
    }),
  contentType: z.literal(PDF_CONTENT_TYPE).optional(),
});

const requestSchema = z.object({
  template: z.string().min(1),
  to: z.array(z.email()).min(1),
  from: z.string().min(1).optional(),
  idempotencyKey: z.string().min(1),
  props: z.record(z.string(), z.unknown()),
  attachments: z.array(attachmentSchema).max(MAX_ATTACHMENT_COUNT).optional(),
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
    console.warn("invalid transactional email envelope", {
      issueCount: parsed.error.issues.length,
    });
    return json({ error: "Invalid transactional email request" }, 400);
  }

  const template = resolveTransactionalEmailTemplate(parsed.data.template);
  if (!template) {
    console.warn("unknown transactional email template", {
      template: parsed.data.template,
      recipientCount: parsed.data.to.length,
      attachmentCount: parsed.data.attachments?.length ?? 0,
    });
    return json({ error: "Unknown transactional email template" }, 400);
  }

  const props = template.schema.safeParse(parsed.data.props);
  if (!props.success) {
    console.warn("invalid transactional email template props", {
      template: parsed.data.template,
      recipientCount: parsed.data.to.length,
      attachmentCount: parsed.data.attachments?.length ?? 0,
      issueCount: props.error.issues.length,
    });
    return json({ error: "Invalid transactional email template props" }, 400);
  }

  const response = await getResend().emails.send(
    {
      from: parsed.data.from ?? defaultFromEmail(),
      to: parsed.data.to,
      subject: template.subject(props.data),
      react: template.render(props.data),
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
    console.error("transactional email provider failure", {
      template: parsed.data.template,
      recipientCount: parsed.data.to.length,
      attachmentCount: parsed.data.attachments?.length ?? 0,
      error: response.error.message,
    });
    captureException(
      response.error instanceof Error
        ? response.error
        : new Error(response.error.message),
      {
        tags: {
          feature: "transactional-email",
          template: parsed.data.template,
        },
        extra: {
          recipientCount: parsed.data.to.length,
          attachmentCount: parsed.data.attachments?.length ?? 0,
        },
      }
    );
    return json({ error: response.error.message }, 502);
  }

  console.info("transactional email sent", {
    template: parsed.data.template,
    recipientCount: parsed.data.to.length,
    attachmentCount: parsed.data.attachments?.length ?? 0,
    resendId: response.data?.id ?? null,
  });

  return json({ id: response.data?.id ?? null });
}
