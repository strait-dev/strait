import { Loading03Icon, UserMultipleIcon } from "@hugeicons/core-free-icons";
import { HugeiconsIcon } from "@hugeicons/react";
import { Button } from "@strait/ui/components/button";
import { toast } from "@strait/ui/components/toast/index";
import { createFileRoute, redirect } from "@tanstack/react-router";
import { createServerFn } from "@tanstack/react-start";
import { getRequestHeaders } from "@tanstack/react-start/server";
import { zodValidator } from "@tanstack/zod-adapter";
import { useCallback, useEffect, useState } from "react";
import z from "zod/v4";
import { AuthLayout } from "@/components/(auth)/auth-layout";
import {
  getPublicInvitationServerFn,
  type PublicInvitationData,
} from "@/hooks/auth/use-invitation";
import { auth } from "@/lib/auth.server";
import { authClient } from "@/lib/auth-client";
import { captureException, captureSentryAuthError } from "@/lib/sentry";

const getSessionServerFn = createServerFn({ method: "GET" }).handler(
  async () => {
    try {
      const headers = getRequestHeaders();
      const session = await auth.api.getSession({ headers });
      return session ?? null;
    } catch {
      return null;
    }
  }
);

const searchParamsSchema = z.object({
  error: z.string().optional(),
});

type SessionData = Awaited<ReturnType<typeof getSessionServerFn>>;

export const Route = createFileRoute("/invitation/$id")({
  validateSearch: zodValidator(searchParamsSchema),
  beforeLoad: async () => {
    const session = await getSessionServerFn();
    return { session };
  },
  loader: async ({
    params,
    context,
  }): Promise<{
    invitation: PublicInvitationData;
    session: SessionData;
  }> => {
    const { session } = context;

    try {
      const invitation = await getPublicInvitationServerFn({
        data: { id: params.id },
      });

      if (!invitation) {
        throw redirect({
          to: "/login",
          search: { error: "Invitation not found or expired" },
        });
      }

      if (invitation.status !== "pending") {
        const errorMessage =
          invitation.status === "cancelled"
            ? "This invitation has been cancelled"
            : "This invitation has already been used";
        throw redirect({
          to: "/login",
          search: { error: errorMessage },
        });
      }

      if (new Date() > new Date(invitation.expiresAt)) {
        throw redirect({
          to: "/login",
          search: { error: "This invitation has expired" },
        });
      }

      return {
        invitation,
        session,
      };
    } catch (error) {
      if ((error as { status?: number })?.status === 302) {
        throw error;
      }
      captureException(error);
      throw redirect({
        to: "/login",
        search: { error: "Error loading invitation" },
      });
    }
  },
  component: RouteComponent,
});

function RouteComponent() {
  const loaderData = Route.useLoaderData();
  const { error } = Route.useSearch();

  const { invitation, session } = loaderData;
  const navigate = Route.useNavigate();
  const [isAccepting, setIsAccepting] = useState(false);
  const [acceptingError, setAcceptingError] = useState<string | null>(null);
  const [isSigningOut, setIsSigningOut] = useState(false);
  const [isGooglePending, setIsGooglePending] = useState(false);

  const handleAcceptInvitation = useCallback(async () => {
    if (!session?.user) {
      navigate({
        to: "/login",
        search: {
          redirect: `/invitation/${invitation.id}`,
        },
      });
      return;
    }

    setIsAccepting(true);
    setAcceptingError(null);

    try {
      await authClient.organization.acceptInvitation(
        {
          invitationId: invitation.id,
        },
        {
          onSuccess: () => {
            toast.success("Invitation accepted successfully!");
            navigate({
              to: "/app",
              search: {
                subscription: undefined,
                t: undefined,
              },
            });
          },
          onError: (ctx) => {
            captureException(ctx.error);
            setAcceptingError(
              ctx.error.message ||
                "Error accepting invitation. Please try again."
            );
          },
        }
      );
    } catch (err) {
      captureException(err);
      setAcceptingError(
        err instanceof Error
          ? err.message
          : "Error accepting invitation. Please try again."
      );
    } finally {
      setIsAccepting(false);
    }
  }, [invitation.id, session?.user, navigate]);

  const handleRejectInvitation = useCallback(async () => {
    try {
      await authClient.organization.rejectInvitation(
        {
          invitationId: invitation.id,
        },
        {
          onSuccess: () => {
            toast.success("Invitation rejected");
            navigate({ to: "/login" });
          },
          onError: (ctx) => {
            captureException(ctx.error);
            toast.error("Error rejecting invitation");
          },
        }
      );
    } catch (err) {
      captureException(err);
      toast.error("Error rejecting invitation");
    }
  }, [invitation.id, navigate]);

  const onGoogleSignIn = async () => {
    try {
      setIsGooglePending(true);
      const callbackURL = `/invitation/${invitation.id}`;

      await authClient.signIn.social(
        {
          provider: "google",
          callbackURL,
        },
        {
          onSuccess: () => {
            setIsGooglePending(false);
          },
          onError: (ctx) => {
            setIsGooglePending(false);
            captureSentryAuthError(ctx.error, {
              operation: "google-oauth",
              provider: "google",
            });
            toast.error("Failed to sign in with Google. Please try again.");
          },
        }
      );
    } catch (err) {
      setIsGooglePending(false);
      captureSentryAuthError(err, {
        operation: "google-oauth",
        provider: "google",
      });
      toast.error("Failed to sign in with Google. Please try again.");
    }
  };

  useEffect(() => {
    if (error) {
      toast.error(error);
    }
  }, [error]);

  // If user is not authenticated, show the Google sign-in option
  if (!session?.user) {
    return (
      <AuthLayout title="Accept Invitation">
        <div className="flex flex-col gap-6">
          <div className="flex flex-col items-center gap-4 text-center">
            <div className="rounded-full bg-primary/10 p-3">
              <HugeiconsIcon
                className="h-8 w-8 text-primary"
                icon={UserMultipleIcon}
              />
            </div>
            <div>
              <h2 className="font-normal text-lg">
                You have been invited to join{" "}
                {invitation.organization?.name || "a store"}
              </h2>
              <p className="text-muted-foreground text-sm">
                Invited by:{" "}
                {invitation.inviter?.name || invitation.inviter?.email}
              </p>
            </div>
          </div>

          <p className="text-center text-muted-foreground text-sm">
            Sign in with Google to accept this invitation.
          </p>

          <Button
            disabled={isGooglePending}
            onClick={onGoogleSignIn}
            type="button"
          >
            {isGooglePending ? (
              <>
                <HugeiconsIcon
                  className="size-4 shrink-0 animate-spin"
                  icon={Loading03Icon}
                />
                <span>Signing in...</span>
              </>
            ) : (
              <>
                <img
                  alt="Google Logo"
                  className="size-4 shrink-0"
                  height={16}
                  loading="lazy"
                  src="/strait.svg"
                  width={16}
                />
                <span>Continue with Google</span>
              </>
            )}
          </Button>
        </div>
      </AuthLayout>
    );
  }

  // User is authenticated, show accept/reject options
  return (
    <AuthLayout title="Accept Invitation">
      <div className="flex flex-col gap-6">
        <div className="flex flex-col items-center gap-4 text-center">
          <div className="rounded-full bg-primary/10 p-3">
            <HugeiconsIcon
              className="h-8 w-8 text-primary"
              icon={UserMultipleIcon}
            />
          </div>
          <div>
            <h2 className="font-normal text-lg">
              You have been invited to join{" "}
              {invitation.organization?.name || "a store"}
            </h2>
            <p className="text-muted-foreground text-sm">
              Invited by:{" "}
              {invitation.inviter?.name || invitation.inviter?.email}
            </p>
            {invitation.role ? (
              <p className="text-muted-foreground text-sm">
                Role: {invitation.role}
              </p>
            ) : null}
          </div>
        </div>

        {acceptingError ? (
          <div className="rounded-md border border-red-200 bg-red-50 p-3">
            <p className="text-red-600 text-sm">{acceptingError}</p>
          </div>
        ) : null}

        <div className="flex flex-col gap-3">
          <Button
            disabled={isAccepting}
            onClick={handleAcceptInvitation}
            type="button"
          >
            {isAccepting ? (
              <>
                <HugeiconsIcon
                  className="size-4 animate-spin"
                  icon={Loading03Icon}
                />
                Accepting invitation...
              </>
            ) : (
              "Accept invitation"
            )}
          </Button>

          <Button
            disabled={isAccepting}
            onClick={handleRejectInvitation}
            type="button"
            variant="secondary"
          >
            Reject invitation
          </Button>
        </div>

        <div className="text-center">
          <p className="text-muted-foreground text-sm">
            Not you?{" "}
            <button
              className="inline-flex items-center gap-1 text-primary hover:underline disabled:cursor-not-allowed disabled:opacity-50"
              disabled={isSigningOut}
              onClick={async () => {
                setIsSigningOut(true);
                try {
                  await authClient.signOut({
                    fetchOptions: {
                      onSuccess: () => {
                        navigate({ to: "/login" });
                      },
                      onError: (ctx) => {
                        captureException(ctx.error);
                        toast.error("Error signing out. Please try again.");
                      },
                    },
                  });
                } catch (err) {
                  captureException(err);
                  toast.error("Error signing out. Please try again.");
                } finally {
                  setIsSigningOut(false);
                }
              }}
              type="button"
            >
              {isSigningOut ? (
                <HugeiconsIcon
                  className="size-3 animate-spin"
                  icon={Loading03Icon}
                />
              ) : null}
              {isSigningOut
                ? "Signing out..."
                : "Sign out and use another account"}
            </button>
          </p>
        </div>
      </div>
    </AuthLayout>
  );
}
