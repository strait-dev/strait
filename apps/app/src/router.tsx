import {
  MutationCache,
  QueryCache,
  QueryClient,
  QueryClientProvider,
} from "@tanstack/react-query";
import { createRouter } from "@tanstack/react-router";
import { setupRouterSsrQueryIntegration } from "@tanstack/react-router-ssr-query";
import ErrorComponent from "@/components/common/error-component";
import NotFound from "@/components/common/not-found";
import { PostHogProvider } from "@/components/providers/posthog-provider";
import { captureException, initializeSentry } from "@/lib/sentry";
import { routeTree } from "./routeTree.gen";

const MAX_QUERY_RETRIES = 3;
const HTTP_STATUS_PATTERN = /failed \((\d{3})\):/;

function getErrorStatus(error: unknown): number | undefined {
  if (error && typeof error === "object" && "cause" in error) {
    const cause = (error as { cause?: unknown }).cause;
    if (
      cause &&
      typeof cause === "object" &&
      "status" in cause &&
      typeof (cause as { status?: unknown }).status === "number"
    ) {
      return (cause as { status: number }).status;
    }
  }

  if (error instanceof Error) {
    const match = HTTP_STATUS_PATTERN.exec(error.message);
    if (match?.[1]) {
      return Number.parseInt(match[1], 10);
    }
  }
}

function shouldRetryQuery(failureCount: number, error: unknown): boolean {
  const status = getErrorStatus(error);
  if (status && status >= 400 && status < 500) {
    return false;
  }
  return failureCount < MAX_QUERY_RETRIES;
}

export const getRouter = () => {
  const queryClient: QueryClient = new QueryClient({
    queryCache: new QueryCache({
      onError: (error, query) => {
        captureException(error, {
          tags: {
            location: "query",
            queryKey: JSON.stringify(query.queryKey),
          },
        });
      },
    }),
    mutationCache: new MutationCache({
      onError: (error, _variables, _context, mutation) => {
        captureException(error, {
          tags: {
            location: "mutation",
            mutationKey: mutation.options.mutationKey
              ? JSON.stringify(mutation.options.mutationKey)
              : undefined,
          },
        });
      },
    }),
    defaultOptions: {
      queries: {
        retry: shouldRetryQuery,
        staleTime: 5 * 60 * 1000,
      },
    },
  });

  const router = createRouter({
    routeTree,
    context: {
      queryClient,
      isAuthenticated: false,
      session: null,
    },
    scrollRestoration: true,
    defaultStructuralSharing: true,
    defaultNotFoundComponent: NotFound,
    defaultErrorComponent: ({ error }) => <ErrorComponent error={error} />,
    Wrap: ({ children }) => (
      <QueryClientProvider client={queryClient}>
        <PostHogProvider>{children}</PostHogProvider>
      </QueryClientProvider>
    ),
  });

  setupRouterSsrQueryIntegration({
    router,
    queryClient,
    wrapQueryClient: false,
  });

  initializeSentry(router);

  return router;
};

// Register the router instance for type safety
declare module "@tanstack/react-router" {
  interface Register {
    router: ReturnType<typeof getRouter>;
  }
}
