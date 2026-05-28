import { Resend } from "resend";

/**
 * Lazily initialized Resend client singleton.
 *
 * Initialization is deferred so tests and deployments can inject env before
 * the first request.
 */
let _resend: Resend | null = null;

export function getResend(): Resend {
  if (!_resend) {
    _resend = new Resend(process.env.RESEND_API_KEY);
  }
  return _resend;
}
