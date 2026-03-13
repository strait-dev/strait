import type { ErrorComponentProps } from "@tanstack/react-router";
import {
  ErrorComponent,
  Link,
  rootRouteId,
  useMatch,
  useRouter,
} from "@tanstack/react-router";
import { useEffect } from "react";
import { captureException } from "@/lib/sentry.ts";

export const DefaultCatchBoundary = ({ error }: ErrorComponentProps) => {
  const router = useRouter();
  const isRoot = useMatch({
    strict: false,
    select: (state) => state.id === rootRouteId,
  });

  useEffect(() => {
    captureException(error, {
      tags: {
        location: "catch_boundary",
        is_root: String(isRoot),
      },
    });
  }, [error, isRoot]);

  return (
    <div className="flex min-w-0 flex-1 flex-col items-center justify-center gap-6 p-4">
      <ErrorComponent error={error} />
      <div className="flex flex-wrap items-center gap-2">
        <button
          className="rounded bg-gray-600 px-2 py-1 font-extrabold text-white uppercase dark:bg-gray-700"
          onClick={() => {
            router.invalidate();
          }}
          type="button"
        >
          Try Again
        </button>
        {isRoot ? (
          <Link
            className="rounded bg-gray-600 px-2 py-1 font-extrabold text-white uppercase dark:bg-gray-700"
            to="/"
          >
            Home
          </Link>
        ) : (
          <Link
            className="rounded bg-gray-600 px-2 py-1 font-extrabold text-white uppercase dark:bg-gray-700"
            onClick={(e: React.MouseEvent<HTMLAnchorElement>) => {
              e.preventDefault();
              window.history.back();
            }}
            to="/"
          >
            Go Back
          </Link>
        )}
      </div>
    </div>
  );
};
