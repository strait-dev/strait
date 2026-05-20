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
import { toast } from "@strait/ui/components/toast/index";
import { useQueryClient } from "@tanstack/react-query";
import { useNavigate, useRouter } from "@tanstack/react-router";
import { useEffect, useState } from "react";
import type { OrganizationData } from "@/hooks/auth/use-organization";
import {
  useDeleteLastOrganizationWithToken,
  useDeleteOrganizationWithToken,
  useOrganizations,
  useRequestOrganizationDeletion,
  useResendOrganizationDeletionCode,
  useVerifyOrganizationDeletion,
} from "@/hooks/auth/use-organization";
import { AlertIcon, LoadingIcon, RefreshIcon, TrashIcon } from "@/lib/icons";

type Props = {
  organizationId: string;
  organizationName: string;
};

type Step = "confirm" | "verify" | "deleting";

const DeleteOrganization = ({ organizationId, organizationName }: Props) => {
  const [isOpen, setIsOpen] = useState(false);
  const [step, setStep] = useState<Step>("confirm");
  const [confirmName, setConfirmName] = useState("");
  const [verificationCode, setVerificationCode] = useState("");
  const [resendCooldown, setResendCooldown] = useState(0);

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
    const timer = setTimeout(() => setResendCooldown((c) => c - 1), 1000);
    return () => clearTimeout(timer);
  }, [resendCooldown]);

  const handleOpen = () => {
    setStep("confirm");
    setConfirmName("");
    setVerificationCode("");
    setResendCooldown(0);
    setIsOpen(true);
  };

  const handleClose = () => {
    setIsOpen(false);
    setStep("confirm");
    setConfirmName("");
    setVerificationCode("");
    setResendCooldown(0);
  };

  const handleRequestDeletion = async () => {
    if (confirmName !== organizationName) {
      toast.error("Organization name does not match.");
      return;
    }

    try {
      const result = await requestDeletion.mutateAsync({ organizationId });
      toast.success("Verification code sent to your email.");
      setResendCooldown(result.cooldownRemaining ?? 60);
      setStep("verify");
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
      setResendCooldown(result.cooldownRemaining ?? 60);
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
      setStep("deleting");
      const result = await verifyDeletion.mutateAsync({
        organizationId,
        verificationCode,
      });

      if (!result.verificationToken) {
        toast.error("Failed to verify code. Please try again.");
        setStep("verify");
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
      setStep("verify");
    }
  };

  const otherOrgs = (orgsData?.page ?? []).filter(
    (org: OrganizationData) => org.id !== organizationId
  );
  const isLastOrg = otherOrgs.length === 0;

  return (
    <Card>
      <CardHeader>
        <CardTitle>Delete Organization</CardTitle>
        <CardDescription>
          Permanently delete this organization and all associated data.
        </CardDescription>
      </CardHeader>
      <CardContent>
        <Alert variant="destructive">
          <HugeiconsIcon className="size-4" icon={AlertIcon} />
          <AlertDescription>
            {isLastOrg
              ? "Warning: This is your only organization. Deleting it will reset your workspace and return you to onboarding."
              : "Warning: This action is irreversible and all organization data will be permanently lost."}
          </AlertDescription>
        </Alert>
      </CardContent>
      <CardFooter className="flex justify-end">
        <AlertDialog
          onOpenChange={(open) => !open && handleClose()}
          open={isOpen}
        >
          <Button onClick={handleOpen} variant="destructive">
            <HugeiconsIcon className="size-4" icon={TrashIcon} />
            Delete Organization
          </Button>
          <AlertDialogContent>
            {step === "confirm" && (
              <>
                <AlertDialogHeader>
                  <AlertDialogTitle>Delete Organization</AlertDialogTitle>
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
                      onChange={(e) => setConfirmName(e.target.value)}
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
                      className="w-fit bg-destructive text-destructive-foreground hover:bg-destructive/90"
                      disabled={
                        confirmName !== organizationName ||
                        requestDeletion.isPending
                      }
                      onClick={(e) => {
                        e.preventDefault();
                        handleRequestDeletion();
                      }}
                    >
                      {requestDeletion.isPending ? (
                        <HugeiconsIcon
                          className="size-4 animate-spin"
                          icon={LoadingIcon}
                        />
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
                  <AlertDialogTitle>Enter Verification Code</AlertDialogTitle>
                  <AlertDialogDescription>
                    A verification code has been sent to your email. Enter it
                    below to confirm deletion.
                  </AlertDialogDescription>
                </AlertDialogHeader>
                <div className="py-2">
                  <Field>
                    <FieldLabel>Verification Code</FieldLabel>
                    <Input
                      maxLength={6}
                      onChange={(e) => setVerificationCode(e.target.value)}
                      placeholder="Enter 6-digit code"
                      value={verificationCode}
                    />
                  </Field>
                  <Button
                    className="mt-2 h-auto gap-1 p-0 text-muted-foreground text-xs hover:text-foreground"
                    disabled={resendCooldown > 0 || resendCode.isPending}
                    onClick={handleResendCode}
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
                      className="w-fit bg-destructive text-destructive-foreground hover:bg-destructive/90"
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
                    >
                      {isPending ? (
                        <HugeiconsIcon
                          className="size-4 animate-spin"
                          icon={LoadingIcon}
                        />
                      ) : (
                        <HugeiconsIcon className="size-4" icon={TrashIcon} />
                      )}
                      Delete Organization
                    </AlertDialogAction>
                  </div>
                </AlertDialogFooter>
              </>
            )}
            {step === "deleting" && (
              <>
                <AlertDialogHeader>
                  <AlertDialogTitle>Deleting Organization...</AlertDialogTitle>
                  <AlertDialogDescription>
                    Please wait while we delete your organization.
                  </AlertDialogDescription>
                </AlertDialogHeader>
                <div className="flex items-center justify-center py-6">
                  <HugeiconsIcon
                    className="size-6 animate-spin"
                    icon={LoadingIcon}
                  />
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
