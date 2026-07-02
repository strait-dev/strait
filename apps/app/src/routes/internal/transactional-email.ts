import { createFileRoute } from "@tanstack/react-router";
import { handleTransactionalEmailRequest } from "@/lib/transactional-email.server";

export const Route = createFileRoute("/internal/transactional-email")({
  server: {
    handlers: {
      POST: ({ request }) => handleTransactionalEmailRequest(request),
    },
  },
});
