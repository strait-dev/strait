import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createRouter } from "@tanstack/react-router";
import { routerWithQueryClient } from "@tanstack/react-router-with-query";
import ErrorComponent from "@/components/common/error-component";
import NotFound from "@/components/common/not-found";
import { initializeSentry } from "@/lib/sentry";
import { routeTree } from "./routeTree.gen";

export const getRouter = () => {
  const queryClient: QueryClient = new QueryClient({
    defaultOptions: {
      queries: {
        staleTime: 5 * 60 * 1000,
      },
    },
  });

  const router = routerWithQueryClient(
    createRouter({
      routeTree,
      context: {
        queryClient,
        isAuthenticated: false,
        session: null,
      },
      defaultPreload: "intent",
      defaultPreloadStaleTime: 0,
      scrollRestoration: true,
      defaultStructuralSharing: true,
      defaultNotFoundComponent: NotFound,
      defaultErrorComponent: ({ error }) => <ErrorComponent error={error} />,
      Wrap: ({ children }) => (
        <QueryClientProvider client={queryClient}>
          {children}
        </QueryClientProvider>
      ),
    }),
    queryClient
  );

  initializeSentry(router);

  return router;
};

// Register the router instance for type safety
declare module "@tanstack/react-router" {
  interface Register {
    router: ReturnType<typeof getRouter>;
  }
}
