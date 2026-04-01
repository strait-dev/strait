import { Resend } from "resend";

/**
 * Lazily initialized Resend client singleton.
 *
 * Initialization is deferred because Cloudflare Workers only populate
 * `process.env` during request handling, not at module load time. Calling
 * `new Resend(process.env.RESEND_API_KEY)` at the top level would receive
 * `undefined` and throw.
 */
let _resend: Resend | null = null;

export function getResend(): Resend {
  if (!_resend) {
    _resend = new Resend(process.env.RESEND_API_KEY);
  }
  return _resend;
}
