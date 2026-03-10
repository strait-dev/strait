import { createFileRoute, redirect } from "@tanstack/react-router";

export const Route = createFileRoute("/")({
  beforeLoad: ({ context }) => {
    // If authenticated, redirect to /app (dashboard)
    if (context.isAuthenticated) {
      throw redirect({ to: "/app" });
    }
    // For unauthenticated users, redirect to login page
    throw redirect({ to: "/login" });
  },
  component: () => null,
});
