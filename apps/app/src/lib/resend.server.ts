import { Resend } from "resend";
import {
  appendLocalEmail,
  shouldUseLocalEmailOutbox,
} from "@/lib/local-email-outbox.server";

/**
 * Lazily initialized Resend client singleton.
 *
 * Initialization is deferred so tests and deployments can inject env before
 * the first request.
 */
let _resend: Resend | null = null;

export function getResend(): Resend {
  if (!_resend) {
    if (shouldUseLocalEmailOutbox()) {
      _resend = {
        emails: {
          send: (message) => {
            const email = appendLocalEmail({
              from: message.from,
              to: message.to,
              subject: message.subject,
              html: message.html,
            });

            return Promise.resolve({ data: { id: email.id }, error: null });
          },
        },
      } as Resend;
      return _resend;
    }

    _resend = new Resend(process.env.RESEND_API_KEY);
  }
  return _resend;
}
