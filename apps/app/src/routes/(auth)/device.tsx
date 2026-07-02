import { HugeiconsIcon } from "@hugeicons/react";
import { Alert, AlertDescription } from "@strait/ui/components/alert";
import { Button } from "@strait/ui/components/button";
import { EmptyMedia } from "@strait/ui/components/empty";
import { Kbd } from "@strait/ui/components/kbd";
import { Spinner } from "@strait/ui/components/spinner";
import { createFileRoute, redirect } from "@tanstack/react-router";
import { createServerFn } from "@tanstack/react-start";
import { useState } from "react";
import { z } from "zod";
import AuthLayout from "@/components/(auth)/auth-layout";
import ErrorComponent from "@/components/common/error-component";
import NotFound from "@/components/common/not-found";
import { apiRequest } from "@/lib/api-client.server";
import { CheckCircleIcon } from "@/lib/icons";
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
        title="Device authorization"
      >
        <p className="text-center text-muted-foreground text-sm">
          No authorization code provided. Run <Kbd>strait login</Kbd> in your
          terminal to get started.
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
        title="Device authorized"
      >
        <div className="flex flex-col items-center gap-3">
          <EmptyMedia media="icon" size="lg" variant="success">
            <HugeiconsIcon className="size-6" icon={CheckCircleIcon} />
          </EmptyMedia>
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
          <Kbd className="px-6 py-3 font-bold text-2xl" size="lg">
            {code}
          </Kbd>
        </div>

        {error ? (
          <Alert className="w-full" variant="destructive">
            <AlertDescription>{error}</AlertDescription>
          </Alert>
        ) : null}

        <div className="flex w-full gap-3">
          <Button
            className="flex-1"
            onClick={() => window.close()}
            type="button"
            variant="secondary-outline"
          >
            Deny
          </Button>
          <Button
            className="flex-1"
            disabled={status === "approving"}
            onClick={handleApprove}
            type="button"
            variant="brand-solid"
          >
            {status === "approving" ? (
              <>
                <Spinner />
                Authorizing...
              </>
            ) : (
              "Authorize"
            )}
          </Button>
        </div>

        <p className="text-center text-muted-foreground text-xs">
          This will create an API key for CLI access with standard permissions.
        </p>
      </div>
    </AuthLayout>
  );
}
