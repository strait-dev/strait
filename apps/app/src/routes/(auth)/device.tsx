import { createFileRoute, redirect } from "@tanstack/react-router";
import { createServerFn } from "@tanstack/react-start";
import { useState } from "react";
import { z } from "zod";
import AuthLayout from "@/components/(auth)/auth-layout";
import ErrorComponent from "@/components/common/error-component";
import NotFound from "@/components/common/not-found";
import { apiRequest } from "@/lib/api-client.server";
import { authMiddleware } from "@/middlewares/auth";
import { requireActiveProjectAccess } from "@/middlewares/require-access";

const deviceSearchSchema = z.object({
  code: z.string().optional().catch(undefined),
});

type ApproveResponse = {
  status: string;
};

const approveDeviceCode = createServerFn({ method: "POST" })
  .inputValidator(
    z.object({
      userCode: z.string().min(1),
    })
  )
  .middleware([authMiddleware])
  .handler(async ({ context, data }) => {
    const projectId = await requireActiveProjectAccess(context);
    return await apiRequest<ApproveResponse>("/v1/cli/device-codes/approve", {
      method: "POST",
      projectId,
      body: {
        project_id: projectId,
        user_code: data.userCode,
      },
    });
  });

export const Route = createFileRoute("/(auth)/device")({
  validateSearch: deviceSearchSchema,
  beforeLoad: ({ context, search }) => {
    if (!context.isAuthenticated) {
      throw redirect({
        to: "/login",
        search: {
          redirect: `/device${search.code ? `?code=${search.code}` : ""}`,
        },
      });
    }
  },
  errorComponent: ErrorComponent,
  notFoundComponent: NotFound,
  component: DeviceAuthPage,
});

function DeviceAuthPage() {
  const { code } = Route.useSearch();
  const [status, setStatus] = useState<
    "idle" | "approving" | "approved" | "error"
  >("idle");
  const [error, setError] = useState<string | null>(null);

  if (!code) {
    return (
      <AuthLayout
        description="Enter the code shown in your terminal to authorize the CLI."
        title="Device Authorization"
      >
        <p className="text-center text-muted-foreground text-sm">
          No authorization code provided. Run{" "}
          <code className="rounded bg-muted px-1.5 py-0.5 font-mono text-xs">
            strait login
          </code>{" "}
          in your terminal to get started.
        </p>
      </AuthLayout>
    );
  }

  async function handleApprove() {
    if (!code) {
      return;
    }
    setStatus("approving");
    setError(null);

    try {
      await approveDeviceCode({ data: { userCode: code } });
      setStatus("approved");
    } catch (err) {
      setStatus("error");
      setError(
        err instanceof Error ? err.message : "Failed to authorize device"
      );
    }
  }

  if (status === "approved") {
    return (
      <AuthLayout
        description="You can close this window and return to your terminal."
        title="Device Authorized"
      >
        <div className="flex flex-col items-center gap-3">
          <div className="flex size-12 items-center justify-center rounded-full bg-success/10">
            <svg
              className="size-6 text-success"
              fill="none"
              stroke="currentColor"
              strokeWidth={2}
              viewBox="0 0 24 24"
            >
              <path
                d="M5 13l4 4L19 7"
                strokeLinecap="round"
                strokeLinejoin="round"
              />
            </svg>
          </div>
          <p className="text-center text-muted-foreground text-sm">
            The Strait CLI has been authorized successfully.
            <br />
            You can close this tab and return to your terminal.
          </p>
        </div>
      </AuthLayout>
    );
  }

  return (
    <AuthLayout
      description="A device is requesting access to your Strait account."
      title="Authorize CLI"
    >
      <div className="flex flex-col items-center gap-4">
        <div className="flex flex-col items-center gap-1">
          <p className="text-muted-foreground text-sm">
            Confirm this code matches your terminal:
          </p>
          <div className="rounded-lg border-2 border-border bg-muted/50 px-6 py-3">
            <span className="font-bold font-mono text-2xl text-foreground">
              {code}
            </span>
          </div>
        </div>

        {error ? (
          <div
            className="w-full rounded-md bg-destructive/10 p-3 text-destructive text-sm"
            role="alert"
          >
            {error}
          </div>
        ) : null}

        <div className="flex w-full gap-3">
          <button
            className="flex-1 rounded border border-border bg-background px-4 py-2.5 font-medium text-foreground text-sm transition-colors hover:bg-muted"
            onClick={() => window.close()}
            type="button"
          >
            Deny
          </button>
          <button
            className="flex-1 rounded bg-primary px-4 py-2.5 font-medium text-primary-foreground text-sm transition-colors hover:bg-primary/90 disabled:opacity-50"
            disabled={status === "approving"}
            onClick={handleApprove}
            type="button"
          >
            {status === "approving" ? "Authorizing..." : "Authorize"}
          </button>
        </div>

        <p className="text-center text-muted-foreground text-xs">
          This will create an API key for CLI access with standard permissions.
        </p>
      </div>
    </AuthLayout>
  );
}
