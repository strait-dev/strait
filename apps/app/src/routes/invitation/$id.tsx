import { HugeiconsIcon } from "@hugeicons/react";
import { Alert, AlertDescription } from "@strait/ui/components/alert";
import { Button } from "@strait/ui/components/button";
import { EmptyMedia } from "@strait/ui/components/empty";
import { Spinner } from "@strait/ui/components/spinner";
import { toast } from "@strait/ui/components/toast";
import { createFileRoute, redirect } from "@tanstack/react-router";
import { zodValidator } from "@tanstack/zod-adapter";
import { useEffect, useState } from "react";
import z from "zod/v4";
import AuthLayout from "@/components/(auth)/auth-layout";
import {
  getPublicInvitationServerFn,
  type PublicInvitationData,
} from "@/hooks/auth/use-invitation";
import { authClient } from "@/lib/auth-client";
import { getSession } from "@/lib/auth-handler";
import { GlobeIcon, UsersAltIcon } from "@/lib/icons";
import { captureException, captureSentryAuthError } from "@/lib/sentry";
import { seo } from "@/lib/seo";

const searchParamsSchema = z.object({
  error: z.string().optional(),
});

type SessionData = Awaited<ReturnType<typeof getSession>>;

export const Route = createFileRoute("/invitation/$id")({
  validateSearch: zodValidator(searchParamsSchema),
  beforeLoad: async () => {
    const session = await getSession();
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
  head: () => ({ meta: seo({ title: "Invitation" }) }),
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

  const handleAcceptInvitation = async () => {
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
          onError: (ctx: { error: Error & { message?: string } }) => {
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
    }
    setIsAccepting(false);
  };

  const handleRejectInvitation = async () => {
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
          onError: (ctx: { error: Error & { message?: string } }) => {
            captureException(ctx.error);
            toast.error("Error rejecting invitation");
          },
        }
      );
    } catch (err) {
      captureException(err);
      toast.error("Error rejecting invitation");
    }
  };

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
          onError: (ctx: { error: Error & { message?: string } }) => {
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
      <AuthLayout title="Accept invitation">
        <div className="flex flex-col gap-6">
          <div className="flex flex-col items-center gap-4 text-center">
            <EmptyMedia media="icon" size="lg">
              <HugeiconsIcon
                className="size-6 text-foreground"
                icon={UsersAltIcon}
              />
            </EmptyMedia>
            <div>
              <h2 className="text-balance font-normal text-lg">
                You have been invited to join{" "}
                {invitation.organization?.name || "an organization"}
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
                <Spinner className="shrink-0" />
                <span>Signing in...</span>
              </>
            ) : (
              <>
                <HugeiconsIcon
                  aria-hidden="true"
                  className="size-4 shrink-0"
                  icon={GlobeIcon}
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
    <AuthLayout title="Accept invitation">
      <div className="flex flex-col gap-6">
        <div className="flex flex-col items-center gap-4 text-center">
          <EmptyMedia media="icon" size="lg">
            <HugeiconsIcon
              className="size-6 text-foreground"
              icon={UsersAltIcon}
            />
          </EmptyMedia>
          <div>
            <h2 className="text-balance font-normal text-lg">
              You have been invited to join{" "}
              {invitation.organization?.name || "an organization"}
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
          <Alert variant="destructive">
            <AlertDescription>{acceptingError}</AlertDescription>
          </Alert>
        ) : null}

        <div className="flex flex-col gap-3">
          <Button
            disabled={isAccepting}
            onClick={handleAcceptInvitation}
            type="button"
          >
            {isAccepting ? (
              <>
                <Spinner />
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
            <Button
              disabled={isSigningOut}
              onClick={async () => {
                setIsSigningOut(true);
                try {
                  await authClient.signOut({
                    fetchOptions: {
                      onSuccess: () => {
                        navigate({ to: "/login" });
                      },
                      onError: (ctx: {
                        error: Error & { message?: string };
                      }) => {
                        captureException(ctx.error);
                        toast.error("Error signing out. Please try again.");
                      },
                    },
                  });
                } catch (err) {
                  captureException(err);
                  toast.error("Error signing out. Please try again.");
                }
                setIsSigningOut(false);
              }}
              size="xs"
              variant="link"
            >
              {isSigningOut ? <Spinner size="xs" /> : null}
              {isSigningOut
                ? "Signing out..."
                : "Sign out and use another account"}
            </Button>
          </p>
        </div>
      </div>
    </AuthLayout>
  );
}
