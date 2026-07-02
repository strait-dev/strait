import type { ReactElement } from "react";
import { Resend } from "resend";

type SendTemplateInput = {
  to: string | string[];
  subject: string;
  react: ReactElement;
  from?: string;
  replyTo?: string | string[];
  idempotencyKey?: string;
  attachments?: Array<{
    filename: string;
    content: string;
    contentType?: string;
  }>;
};

type TransactionalEmailSenderConfig = {
  apiKey?: string;
  from: string;
};

export type TransactionalEmailSender = {
  send(input: SendTemplateInput): Promise<{ id: string } | null>;
};

export const createTransactionalEmailSender = ({
  apiKey,
  from,
}: TransactionalEmailSenderConfig): TransactionalEmailSender => {
  const resend = new Resend(apiKey);

  return {
    async send(input) {
      const response = await resend.emails.send(
        {
          from: input.from ?? from,
          to: input.to,
          subject: input.subject,
          react: input.react,
          replyTo: input.replyTo,
          attachments: input.attachments,
        },
        {
          idempotencyKey: input.idempotencyKey,
        }
      );

      if (response.error) {
        throw new Error(response.error.message);
      }

      return response.data;
    },
  };
};
