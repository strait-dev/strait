import { HugeiconsIcon } from "@hugeicons/react";
import { Alert, AlertDescription } from "@strait/ui/components/alert";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@strait/ui/components/alert-dialog";
import { Button } from "@strait/ui/components/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from "@strait/ui/components/card";
import { Field, FieldLabel } from "@strait/ui/components/field";
import { Input } from "@strait/ui/components/input";
import { Spinner } from "@strait/ui/components/spinner";
import { toast } from "@strait/ui/components/toast";
import { useQueryClient } from "@tanstack/react-query";
import { useNavigate, useRouter } from "@tanstack/react-router";
import { useEffect, useReducer } from "react";
import type { OrganizationData } from "@/hooks/auth/use-organization";
import {
  useDeleteLastOrganizationWithToken,
  useDeleteOrganizationWithToken,
  useOrganizations,
  useRequestOrganizationDeletion,
  useResendOrganizationDeletionCode,
  useVerifyOrganizationDeletion,
} from "@/hooks/auth/use-organization";
import { AlertIcon, RefreshIcon, TrashIcon } from "@/lib/icons";

type Props = {
  organizationId: string;
  organizationName: string;
};

type Step = "confirm" | "verify" | "deleting";

type DeleteDialogState = {
  isOpen: boolean;
  step: Step;
  confirmName: string;
  verificationCode: string;
  resendCooldown: number;
};

type DeleteDialogAction =
  | { type: "open" }
  | { type: "close" }
  | { type: "set-confirm-name"; value: string }
  | { type: "set-verification-code"; value: string }
  | { type: "set-step"; step: Step }
  | { type: "set-resend-cooldown"; value: number }
  | { type: "tick-resend-cooldown" };

const initialDeleteDialogState: DeleteDialogState = {
  isOpen: false,
  step: "confirm",
  confirmName: "",
  verificationCode: "",
  resendCooldown: 0,
};

function deleteDialogReducer(
  state: DeleteDialogState,
  action: DeleteDialogAction
): DeleteDialogState {
  switch (action.type) {
    case "open":
      return { ...initialDeleteDialogState, isOpen: true };
    case "close":
      return initialDeleteDialogState;
    case "set-confirm-name":
      return { ...state, confirmName: action.value };
    case "set-verification-code":
      return { ...state, verificationCode: action.value };
    case "set-step":
      return { ...state, step: action.step };
    case "set-resend-cooldown":
      return { ...state, resendCooldown: action.value };
    case "tick-resend-cooldown":
      return {
        ...state,
        resendCooldown: Math.max(0, state.resendCooldown - 1),
      };
    default:
      return state;
  }
}

const DeleteOrganization = ({ organizationId, organizationName }: Props) => {
  const [dialogState, dispatchDialog] = useReducer(
    deleteDialogReducer,
    initialDeleteDialogState
  );
  const { isOpen, step, confirmName, verificationCode, resendCooldown } =
    dialogState;

  const navigate = useNavigate();
  const router = useRouter();
  const queryClient = useQueryClient();
  const requestDeletion = useRequestOrganizationDeletion();
  const verifyDeletion = useVerifyOrganizationDeletion();
  const deleteMutation = useDeleteOrganizationWithToken();
  const deleteLastMutation = useDeleteLastOrganizationWithToken();
  const resendCode = useResendOrganizationDeletionCode();
  const { data: orgsData } = useOrganizations();

  const isPending =
    requestDeletion.isPending ||
    verifyDeletion.isPending ||
    deleteMutation.isPending ||
    deleteLastMutation.isPending;

  useEffect(() => {
    if (resendCooldown <= 0) {
      return;
    }
    const timer = setTimeout(
      () => dispatchDialog({ type: "tick-resend-cooldown" }),
      1000
    );
    return () => clearTimeout(timer);
  }, [resendCooldown]);

  const handleOpen = () => {
    dispatchDialog({ type: "open" });
  };

  const handleClose = () => {
    dispatchDialog({ type: "close" });
  };

  const handleRequestDeletion = async () => {
    if (confirmName !== organizationName) {
      toast.error("Organization name does not match.");
      return;
    }

    try {
      const result = await requestDeletion.mutateAsync({ organizationId });
      toast.success("Verification code sent to your email.");
      dispatchDialog({
        type: "set-resend-cooldown",
        value: result.cooldownRemaining ?? 60,
      });
      dispatchDialog({ type: "set-step", step: "verify" });
    } catch (error) {
      toast.error(
        error instanceof Error
          ? error.message
          : "Failed to request deletion. Please try again."
      );
    }
  };

  const handleResendCode = async () => {
    if (resendCooldown > 0 || resendCode.isPending) {
      return;
    }

    try {
      const result = await resendCode.mutateAsync({ organizationId });
      toast.success("Verification code resent.");
      dispatchDialog({
        type: "set-resend-cooldown",
        value: result.cooldownRemaining ?? 60,
      });
    } catch (error) {
      toast.error(
        error instanceof Error
          ? error.message
          : "Failed to resend code. Please try again."
      );
    }
  };

  const handleVerifyAndDelete = async () => {
    if (verificationCode.length < 6) {
      toast.error("Please enter a valid verification code.");
      return;
    }

    try {
      dispatchDialog({ type: "set-step", step: "deleting" });
      const result = await verifyDeletion.mutateAsync({
        organizationId,
        operation: "delete",
        verificationCode,
      });

      if (!result.verificationToken) {
        toast.error("Failed to verify code. Please try again.");
        dispatchDialog({ type: "set-step", step: "verify" });
        return;
      }

      const otherOrgs = (orgsData?.page ?? []).filter(
        (org: OrganizationData) => org.id !== organizationId
      );

      if (otherOrgs.length === 0) {
        await deleteLastMutation.mutateAsync({
          organizationId,
          verificationToken: result.verificationToken,
        });
      } else {
        await deleteMutation.mutateAsync({
          organizationId,
          verificationToken: result.verificationToken,
          nextOrganizationId: otherOrgs[0].id,
        });
      }

      await queryClient.invalidateQueries();
      router.invalidate();
      toast.success("Organization deleted successfully.");
      handleClose();
      navigate({ to: "/app" });
    } catch (error) {
      toast.error(
        error instanceof Error
          ? error.message
          : "Failed to delete organization. Please try again."
      );
      dispatchDialog({ type: "set-step", step: "verify" });
    }
  };

  const otherOrgs = (orgsData?.page ?? []).filter(
    (org: OrganizationData) => org.id !== organizationId
  );
  const isLastOrg = otherOrgs.length === 0;

  return (
    <Card>
      <CardHeader>
        <CardTitle>Delete organization</CardTitle>
        <CardDescription>
          Permanently delete this organization and all associated data.
        </CardDescription>
      </CardHeader>
      <CardContent className="pb-6">
        <Alert variant="destructive">
          <HugeiconsIcon className="size-4" icon={AlertIcon} />
          <AlertDescription>
            {isLastOrg
              ? "Warning: This is your only organization. Deleting it will reset your workspace and return you to onboarding."
              : "Warning: This action is irreversible and all organization data will be permanently lost."}
          </AlertDescription>
        </Alert>
      </CardContent>
      <CardFooter className="flex justify-end border-t px-6 py-4">
        <AlertDialog
          onOpenChange={(open) => !open && handleClose()}
          open={isOpen}
        >
          <Button onClick={handleOpen} variant="destructive">
            <HugeiconsIcon className="size-4" icon={TrashIcon} />
            Delete organization
          </Button>
          <AlertDialogContent>
            {step === "confirm" && (
              <>
                <AlertDialogHeader>
                  <AlertDialogTitle>Delete organization</AlertDialogTitle>
                  <AlertDialogDescription>
                    This will permanently delete{" "}
                    <strong>{organizationName}</strong> and all its data. Type
                    the organization name to confirm.
                  </AlertDialogDescription>
                </AlertDialogHeader>
                <div className="py-2">
                  <Field>
                    <FieldLabel>
                      Type "{organizationName}" to confirm
                    </FieldLabel>
                    <Input
                      onChange={(e) =>
                        dispatchDialog({
                          type: "set-confirm-name",
                          value: e.target.value,
                        })
                      }
                      placeholder={organizationName}
                      value={confirmName}
                    />
                  </Field>
                </div>
                <AlertDialogFooter>
                  <div className="flex justify-end gap-4">
                    <AlertDialogCancel className="w-fit" onClick={handleClose}>
                      Cancel
                    </AlertDialogCancel>
                    <AlertDialogAction
                      className="w-fit"
                      disabled={
                        confirmName !== organizationName ||
                        requestDeletion.isPending
                      }
                      onClick={(e) => {
                        e.preventDefault();
                        handleRequestDeletion();
                      }}
                      variant="destructive-solid"
                    >
                      {requestDeletion.isPending ? (
                        <Spinner />
                      ) : (
                        <HugeiconsIcon className="size-4" icon={TrashIcon} />
                      )}
                      Continue
                    </AlertDialogAction>
                  </div>
                </AlertDialogFooter>
              </>
            )}
            {step === "verify" && (
              <>
                <AlertDialogHeader>
                  <AlertDialogTitle>Enter verification code</AlertDialogTitle>
                  <AlertDialogDescription>
                    A verification code has been sent to your email. Enter it
                    below to confirm deletion.
                  </AlertDialogDescription>
                </AlertDialogHeader>
                <div className="py-2">
                  <Field>
                    <FieldLabel>Verification code</FieldLabel>
                    <Input
                      maxLength={6}
                      onChange={(e) =>
                        dispatchDialog({
                          type: "set-verification-code",
                          value: e.target.value,
                        })
                      }
                      placeholder="Enter 6-digit code"
                      value={verificationCode}
                    />
                  </Field>
                  <Button
                    className="mt-2"
                    disabled={resendCooldown > 0 || resendCode.isPending}
                    onClick={handleResendCode}
                    size="xs"
                    variant="link"
                  >
                    <HugeiconsIcon icon={RefreshIcon} size={12} />
                    {resendCooldown > 0
                      ? `Resend code in ${resendCooldown}s`
                      : "Resend code"}
                  </Button>
                </div>
                <AlertDialogFooter>
                  <div className="flex justify-end gap-4">
                    <AlertDialogCancel className="w-fit" onClick={handleClose}>
                      Cancel
                    </AlertDialogCancel>
                    <AlertDialogAction
                      className="w-fit"
                      disabled={
                        verificationCode.length < 6 ||
                        verifyDeletion.isPending ||
                        deleteMutation.isPending ||
                        deleteLastMutation.isPending
                      }
                      onClick={(e) => {
                        e.preventDefault();
                        handleVerifyAndDelete();
                      }}
                      variant="destructive-solid"
                    >
                      {isPending ? (
                        <Spinner />
                      ) : (
                        <HugeiconsIcon className="size-4" icon={TrashIcon} />
                      )}
                      Delete organization
                    </AlertDialogAction>
                  </div>
                </AlertDialogFooter>
              </>
            )}
            {step === "deleting" && (
              <>
                <AlertDialogHeader>
                  <AlertDialogTitle>Deleting organization...</AlertDialogTitle>
                  <AlertDialogDescription>
                    Please wait while we delete your organization.
                  </AlertDialogDescription>
                </AlertDialogHeader>
                <div className="flex items-center justify-center py-6">
                  <Spinner size="lg" />
                </div>
              </>
            )}
          </AlertDialogContent>
        </AlertDialog>
      </CardFooter>
    </Card>
  );
};

export default DeleteOrganization;
