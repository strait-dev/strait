import { HugeiconsIcon } from "@hugeicons/react";
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
import { useNavigate } from "@tanstack/react-router";
import { useState } from "react";
import {
  useDeleteOrganizationWithToken,
  useOrganizations,
  useRequestOrganizationDeletion,
  useVerifyOrganizationDeletion,
} from "@/hooks/auth/use-organization";
import { AlertIcon, LoadingIcon, TrashIcon } from "@/lib/icons";

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

  const navigate = useNavigate();
  const requestDeletion = useRequestOrganizationDeletion();
  const verifyDeletion = useVerifyOrganizationDeletion();
  const deleteMutation = useDeleteOrganizationWithToken();
  const { data: orgsData } = useOrganizations();

  const isPending =
    requestDeletion.isPending ||
    verifyDeletion.isPending ||
    deleteMutation.isPending;

  const handleOpen = () => {
    setStep("confirm");
    setConfirmName("");
    setVerificationCode("");
    setIsOpen(true);
  };

  const handleClose = () => {
    setIsOpen(false);
    setStep("confirm");
    setConfirmName("");
    setVerificationCode("");
  };

  const handleRequestDeletion = async () => {
    if (confirmName !== organizationName) {
      toast.error("Organization name does not match.");
      return;
    }

    try {
      await requestDeletion.mutateAsync({ organizationId });
      toast.success("Verification code sent to your email.");
      setStep("verify");
    } catch (error) {
      toast.error(
        error instanceof Error
          ? error.message
          : "Failed to request deletion. Please try again."
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
        (org) => org.id !== organizationId
      );
      const nextOrgId = otherOrgs[0]?.id ?? "";

      await deleteMutation.mutateAsync({
        organizationId,
        verificationToken: result.verificationToken,
        nextOrganizationId: nextOrgId,
      });

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

  return (
    <Card>
      <CardHeader>
        <CardTitle>Delete Organization</CardTitle>
        <CardDescription>
          Permanently delete this organization and all associated data.
        </CardDescription>
      </CardHeader>
      <CardContent>
        <div className="flex items-center gap-2 rounded-custom border border-destructive/50 bg-destructive/5 px-3 py-2 text-destructive text-sm">
          <HugeiconsIcon className="size-4" icon={AlertIcon} />
          <span>
            Warning: This action is irreversible and all organization data will
            be permanently lost.
          </span>
        </div>
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
                        deleteMutation.isPending
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
