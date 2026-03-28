import { Resend } from "resend";

const disableEmailVerification =
  process.env.DISABLE_EMAIL_VERIFICATION === "true";

const shouldUseNoopResend =
  disableEmailVerification ||
  (!process.env.RESEND_API_KEY && process.env.NODE_ENV !== "production");

export const resend = shouldUseNoopResend
  ? {
      emails: {
        send: async () => ({ id: "dev-noop-email" }),
      },
    }
  : new Resend(process.env.RESEND_API_KEY);
