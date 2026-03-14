import { AlertCircleIcon, Loading03Icon } from "@hugeicons/core-free-icons";
import { HugeiconsIcon } from "@hugeicons/react";
import { Button } from "@strait/ui/components/button";
import { Checkbox } from "@strait/ui/components/checkbox";
import {
  CredenzaContent,
  CredenzaDescription,
  CredenzaHeader,
  CredenzaTitle,
} from "@strait/ui/components/credenza";
import { Field, FieldError, FieldLabel } from "@strait/ui/components/field";
import { Input } from "@strait/ui/components/input";
import {
  InputOTP,
  InputOTPGroup,
  InputOTPSlot,
} from "@strait/ui/components/input-otp";
import { toast } from "@strait/ui/components/toast/index";
import { useForm } from "@tanstack/react-form";
import { useNavigate } from "@tanstack/react-router";
import {
  useCallback,
  useEffect,
  useId,
  useMemo,
  useState,
  useTransition,
} from "react";
import type * as z from "zod/v4";
import type { OrganizationsApiResponse } from "@/hooks/auth/use-organization";
import {
  useDeleteLastOrganizationWithToken,
  useDeleteOrganizationWithToken,
  usePurgeOrganizationWithToken,
  useRequestOrganizationDeletion,
  useResendOrganizationDeletionCode,
  useVerifyOrganizationDeletion,
} from "@/hooks/auth/use-organization";

import {
  DeleteOrganizationSchema,
  VerifyCodeDeletionSchema,
} from "@/lib/schema";
import {
  DEFAULT_COOLDOWN_SECONDS,
  DEFAULT_MAX_WAIT,
  DEFAULT_PINCODE_LENGTH,
} from "@/utils/constants";

// Pre-generated slot identifiers to avoid using array index as key
const OTP_SLOT_IDS = Array.from(
  { length: DEFAULT_PINCODE_LENGTH },
  (_, i) => `otp-slot-${i}`
);

type Props = {
  organization_id: string;
  organizations: OrganizationsApiResponse;
  onOpenChange: (open: boolean) => void;
};

const DeleteOrganizationDialog = ({
  organization_id,
  organizations,
  onOpenChange,
}: Props) => {
  const confirmCheckboxId = useId();
  const wordInputId = useId();
  const verificationFormId = useId();
  const verificationCodeInputId = useId();

  const navigate = useNavigate();

  const [currentStep, setCurrentStep] = useState<
    "confirmation" | "verification"
  >("confirmation");
  const [_cooldownError, setCooldownError] = useState<string | null>(null);
  const [cooldownSeconds, setCooldownSeconds] = useState<number>(0);

  const requestDeletionMutation = useRequestOrganizationDeletion();
  const verifyDeletionMutation = useVerifyOrganizationDeletion();
  const resendCodeMutation = useResendOrganizationDeletionCode();
  const deleteOrgWithTokenMutation = useDeleteOrganizationWithToken();
  const purgeOrgWithTokenMutation = usePurgeOrganizationWithToken();
  const deleteLastOrgWithTokenMutation = useDeleteLastOrganizationWithToken();

  const isSingleOrganization = organizations.page.length <= 1;

  const form = useForm({
    defaultValues: {
      confirm: false,
      word: "",
    },
    validators: { onChange: DeleteOrganizationSchema },
  });

  const verificationForm = useForm({
    defaultValues: {
      verificationCode: "",
    },
    validators: { onChange: VerifyCodeDeletionSchema },
  });

  const [_isPendingTransition, startTransition] = useTransition();

  useEffect(() => {
    const storedCooldown = localStorage.getItem(
      `org-delete-cooldown-${organization_id}`
    );

    if (storedCooldown) {
      try {
        const cooldownData = JSON.parse(storedCooldown);
        const expirationTime = cooldownData.expiration;

        if (expirationTime > Date.now()) {
          const remainingSeconds = Math.ceil(
            (expirationTime - Date.now()) / DEFAULT_COOLDOWN_SECONDS
          );
          setCooldownSeconds(remainingSeconds);

          const minutes = Math.ceil(remainingSeconds / 60);
          setCooldownError(
            `Wait ${minutes} ${minutes === 1 ? "minute" : "minutes"} to request a new code`
          );
        } else {
          localStorage.removeItem(`org-delete-cooldown-${organization_id}`);
        }
      } catch (_error) {
        localStorage.removeItem(`org-delete-cooldown-${organization_id}`);
      }
    }
  }, [organization_id]);

  const startCooldown = useCallback(
    (seconds = 60) => {
      const expirationTime = Date.now() + seconds * DEFAULT_COOLDOWN_SECONDS;
      localStorage.setItem(
        `org-delete-cooldown-${organization_id}`,
        JSON.stringify({ expiration: expirationTime })
      );
      setCooldownSeconds(seconds);

      const minutes = Math.ceil(seconds / 60);
      setCooldownError(
        `Wait ${minutes} ${minutes === 1 ? "minute" : "minutes"} to request a new code`
      );
    },
    [organization_id]
  );

  useEffect(() => {
    return () => {
      form.reset();
      verificationForm.reset();
    };
  }, [form, verificationForm]);

  const isCoolingDown = cooldownSeconds > 0;
  useEffect(() => {
    if (!isCoolingDown) {
      setCooldownError(null);
      return;
    }

    const timerId = setInterval(() => {
      setCooldownSeconds((prev: number) => {
        const newValue = prev - 1;
        if (newValue <= 0) {
          setCooldownError(null);
          localStorage.removeItem(`org-delete-cooldown-${organization_id}`);
          return 0;
        }

        const minutes = Math.ceil(newValue / 60);
        setCooldownError(
          `Wait ${minutes} ${minutes === 1 ? "minute" : "minutes"} to request a new code`
        );

        return newValue;
      });
    }, DEFAULT_COOLDOWN_SECONDS);

    return () => clearInterval(timerId);
  }, [isCoolingDown, organization_id]);

  const onSubmit = useCallback(
    (_values: z.input<typeof DeleteOrganizationSchema>) => {
      if (!organization_id) {
        toast.error("Error processing request, please contact support.");
        return;
      }

      toast.promise(
        requestDeletionMutation.mutateAsync({
          organizationId: organization_id,
        }),
        {
          loading: "Sending verification code...",
          success: (result: {
            success: boolean;
            message?: string;
            cooldownRemaining?: number;
          }) => {
            if (result?.success) {
              setCurrentStep("verification");
              startCooldown(result.cooldownRemaining || DEFAULT_MAX_WAIT);
              verificationForm.reset();
              return "Verification code sent successfully!";
            }
            if (result?.cooldownRemaining) {
              startCooldown(result.cooldownRemaining);
              return "You need to wait before requesting a new code.";
            }
            return (
              result?.message ||
              "Could not send the code. Please try again later."
            );
          },
          error: (err: unknown) =>
            (err as Error).message ||
            "Error sending verification code, please try again.",
        }
      );
    },
    [organization_id, requestDeletionMutation, startCooldown, verificationForm]
  );

  const handleSingleOrgDeletion = useCallback(
    async (organizationId: string, verificationToken: string) => {
      await deleteLastOrgWithTokenMutation.mutateAsync({
        organizationId,
        verificationToken,
      });
      toast.success("Organization deleted successfully!");
      onOpenChange(false);
      navigate({
        to: "/app",
        search: { subscription: undefined, t: undefined },
      });
    },
    [deleteLastOrgWithTokenMutation, onOpenChange, navigate]
  );

  const handleMultiOrgDeletion = useCallback(
    async (organizationId: string, verificationToken: string) => {
      const nextOrgId = organizations.page.find(
        (org) => org.id !== organizationId
      )?.id;

      if (!nextOrgId) {
        toast.error("Could not find an alternative organization.");
        return;
      }

      await deleteOrgWithTokenMutation.mutateAsync({
        organizationId,
        verificationToken,
        nextOrganizationId: nextOrgId,
      });
      toast.success("Organization deleted successfully!");
      onOpenChange(false);
      navigate({
        to: "/app",
        search: { subscription: undefined, t: undefined },
      });
    },
    [organizations.page, deleteOrgWithTokenMutation, onOpenChange, navigate]
  );

  const handleVerifyDeletion = useCallback(
    (values: z.input<typeof VerifyCodeDeletionSchema>) => {
      if (!(organization_id && values.verificationCode)) {
        toast.error("Error processing request, please contact support.");
        return;
      }

      const cleanCode = values.verificationCode.trim().replace(/[^0-9]/g, "");

      if (!cleanCode || cleanCode.length !== DEFAULT_PINCODE_LENGTH) {
        toast.error("The verification code must contain 6 numbers.");
        return;
      }

      startTransition(async () => {
        toast.loading("Verifying code...");
        try {
          const result = await verifyDeletionMutation.mutateAsync({
            organizationId: organization_id,
            verificationCode: cleanCode,
          });

          if (result?.success && result.verificationToken) {
            toast.success("Code verified successfully!");

            if (isSingleOrganization) {
              await handleSingleOrgDeletion(
                organization_id,
                result.verificationToken
              );
            } else {
              await handleMultiOrgDeletion(
                organization_id,
                result.verificationToken
              );
            }
            form.reset();
            verificationForm.reset();
          } else {
            toast.error(
              result?.message || "Invalid or expired verification code"
            );
            verificationForm.setFieldValue("verificationCode", "");
          }
        } catch (error) {
          toast.error(
            (error as Error).message || "Error verifying deletion code"
          );
          verificationForm.setFieldValue("verificationCode", "");
        }
      });
    },
    [
      organization_id,
      isSingleOrganization,
      verificationForm,
      verifyDeletionMutation,
      form,
      handleSingleOrgDeletion,
      handleMultiOrgDeletion,
    ]
  );

  const handleOpenChange = useCallback(
    (open: boolean) => {
      if (!open) {
        setCurrentStep("confirmation");
        form.reset();
        verificationForm.reset();
        const storedCooldown = localStorage.getItem(
          `org-delete-cooldown-${organization_id}`
        );
        if (storedCooldown) {
          try {
            const cooldownData = JSON.parse(storedCooldown);
            const expirationTime = cooldownData.expiration;
            if (expirationTime > Date.now()) {
              const remainingSeconds = Math.ceil(
                (expirationTime - Date.now()) / DEFAULT_COOLDOWN_SECONDS
              );
              setCooldownSeconds(remainingSeconds);
            } else {
              localStorage.removeItem(`org-delete-cooldown-${organization_id}`);
              setCooldownSeconds(0);
              setCooldownError(null);
            }
          } catch (_error) {
            localStorage.removeItem(`org-delete-cooldown-${organization_id}`);
            setCooldownSeconds(0);
            setCooldownError(null);
          }
        }
      }
      onOpenChange(open);
    },
    [organization_id, form, verificationForm, onOpenChange]
  );

  const getConfirmationButtonText = useCallback(() => {
    if (requestDeletionMutation.isPending || cooldownSeconds > 0) {
      return (
        <span className="flex items-center gap-1">
          <HugeiconsIcon className="size-4 animate-spin" icon={Loading03Icon} />
          <span>Processing...</span>
        </span>
      );
    }
    return isSingleOrganization ? "Delete permanently" : "Delete organization";
  }, [
    requestDeletionMutation.isPending,
    cooldownSeconds,
    isSingleOrganization,
  ]);

  const getVerificationButtonText = useCallback(() => {
    const isProcessing =
      verifyDeletionMutation.isPending ||
      deleteOrgWithTokenMutation.isPending ||
      purgeOrgWithTokenMutation.isPending ||
      deleteLastOrgWithTokenMutation.isPending;

    if (isProcessing) {
      return (
        <span className="flex items-center gap-1">
          <HugeiconsIcon className="size-4 animate-spin" icon={Loading03Icon} />
          <span>Verifying...</span>
        </span>
      );
    }
    return "Verify code";
  }, [
    verifyDeletionMutation.isPending,
    deleteOrgWithTokenMutation.isPending,
    purgeOrgWithTokenMutation.isPending,
    deleteLastOrgWithTokenMutation.isPending,
  ]);

  const getFooterJSX = useMemo(() => {
    if (currentStep === "confirmation") {
      return (
        <div className="flex flex-row-reverse gap-3">
          <Button
            disabled={
              !form.state.canSubmit ||
              cooldownSeconds > 0 ||
              requestDeletionMutation.isPending
            }
            onClick={() => onSubmit(form.state.values)}
            variant="destructive"
          >
            {getConfirmationButtonText()}
          </Button>
          <Button
            onClick={() => handleOpenChange(false)}
            type="button"
            variant="outline"
          >
            Cancel
          </Button>
        </div>
      );
    }

    return (
      <div className="flex flex-row-reverse flex-wrap items-center gap-3">
        <Button
          disabled={
            !verificationForm.state.values.verificationCode ||
            verificationForm.state.values.verificationCode.length <
              DEFAULT_PINCODE_LENGTH ||
            verifyDeletionMutation.isPending ||
            deleteOrgWithTokenMutation.isPending ||
            purgeOrgWithTokenMutation.isPending ||
            deleteLastOrgWithTokenMutation.isPending
          }
          onClick={() => handleVerifyDeletion(verificationForm.state.values)}
          variant="destructive"
        >
          {getVerificationButtonText()}
        </Button>
        {cooldownSeconds > 0 ? (
          <p className="text-muted-foreground text-sm">
            Wait {cooldownSeconds}s to resend code
          </p>
        ) : (
          <Button
            disabled={resendCodeMutation.isPending}
            onClick={() =>
              resendCodeMutation.mutate({ organizationId: organization_id })
            }
            type="button"
            variant="outline"
          >
            {resendCodeMutation.isPending ? (
              <span className="flex items-center gap-1">
                <HugeiconsIcon
                  className="size-4 animate-spin"
                  icon={Loading03Icon}
                />
                <span>Resending...</span>
              </span>
            ) : (
              "Resend code"
            )}
          </Button>
        )}
      </div>
    );
  }, [
    currentStep,
    form,
    cooldownSeconds,
    handleOpenChange,
    verificationForm,
    verifyDeletionMutation.isPending,
    deleteOrgWithTokenMutation.isPending,
    purgeOrgWithTokenMutation.isPending,
    deleteLastOrgWithTokenMutation.isPending,
    onSubmit,
    handleVerifyDeletion,
    resendCodeMutation,
    organization_id,
    requestDeletionMutation.isPending,
    getConfirmationButtonText,
    getVerificationButtonText,
  ]);

  const confirmationContent = useMemo(
    () => (
      <form
        className="space-y-4"
        onSubmit={(e) => {
          e.preventDefault();
          onSubmit(form.state.values);
        }}
      >
        {isSingleOrganization ? null : (
          <div className="rounded-md border border-destructive/20 bg-destructive/10 px-4 py-3">
            <div className="flex gap-3">
              <HugeiconsIcon
                aria-hidden="true"
                className="mt-0.5 shrink-0 text-destructive opacity-75"
                icon={AlertCircleIcon}
                size={16}
              />
              <div className="grow space-y-1">
                <p className="font-medium text-destructive text-sm">
                  This action cannot be undone.
                </p>
                <p className="text-destructive text-sm opacity-90">
                  {isSingleOrganization
                    ? "This action will clear all your organization data, but keep the organization intact."
                    : "This action will permanently delete all your organization data."}
                </p>
              </div>
            </div>
          </div>
        )}

        {isSingleOrganization ? (
          <div className="rounded-md border border-destructive/20 bg-destructive/10 px-4 py-3">
            <div className="flex gap-3">
              <HugeiconsIcon
                aria-hidden="true"
                className="mt-0.5 shrink-0 text-destructive opacity-75"
                icon={AlertCircleIcon}
                size={16}
              />
              <div className="grow space-y-1">
                <p className="font-medium text-destructive text-sm">
                  ⚠️ WARNING: This action is irreversible and permanent!
                </p>
                <p className="text-destructive text-sm opacity-90">
                  Deleting your only organization will{" "}
                  <strong>permanently delete ALL data</strong> including:
                </p>
                <ul className="mt-2 ml-4 list-disc text-destructive text-sm opacity-90">
                  <li>All products and inventory</li>
                  <li>All customers and suppliers</li>
                  <li>All orders and sales</li>
                  <li>All settings and preferences</li>
                  <li>All financial history</li>
                </ul>
                <p className="mt-2 font-medium text-destructive text-sm">
                  After deletion, you will be redirected to create a new
                  organization.
                </p>
              </div>
            </div>
          </div>
        ) : null}

        <p className="text-muted-foreground text-sm">
          A verification code will be sent to your email to confirm this action.
        </p>

        <div>
          <form.Field name="confirm">
            {(field) => (
              <Field className="flex items-start">
                <div className="flex items-center gap-2">
                  <Checkbox
                    checked={field.state.value}
                    id={confirmCheckboxId}
                    onCheckedChange={(checked) =>
                      field.handleChange(checked === true)
                    }
                  />
                  <FieldLabel htmlFor={confirmCheckboxId}>
                    I understand that this action cannot be undone
                  </FieldLabel>
                </div>
              </Field>
            )}
          </form.Field>
        </div>

        <form.Field name="word">
          {(field) => (
            <Field>
              <FieldLabel htmlFor={wordInputId}>
                Type the word "delete" to confirm
              </FieldLabel>
              <Input
                id={wordInputId}
                onBlur={field.handleBlur}
                onChange={(e) => field.handleChange(e.target.value)}
                placeholder="Type the word 'delete' to confirm"
                type="text"
                value={field.state.value}
              />
              {field.state.meta.errors.length > 0 && (
                <FieldError>{field.state.meta.errors.join(", ")}</FieldError>
              )}
            </Field>
          )}
        </form.Field>
      </form>
    ),
    [form, isSingleOrganization, onSubmit, confirmCheckboxId, wordInputId]
  );

  const verificationContent = useMemo(
    () => (
      <div className="space-y-4">
        <p className="text-muted-foreground text-sm">
          The code is valid for 5 minutes. If you don't receive the email, check
          your spam folder or request a new code.
        </p>
        <form
          className="space-y-4"
          id={verificationFormId}
          onSubmit={(e) => {
            e.preventDefault();
            handleVerifyDeletion(verificationForm.state.values);
          }}
        >
          <verificationForm.Field name="verificationCode">
            {(field) => (
              <Field className="space-y-2">
                <InputOTP
                  containerClassName="flex justify-center gap-3 w-fit items-center self-center"
                  id={verificationCodeInputId}
                  maxLength={DEFAULT_PINCODE_LENGTH}
                  name={field.name}
                  onBlur={field.handleBlur}
                  onChange={(value) => field.handleChange(value)}
                  value={field.state.value ?? ""}
                >
                  <InputOTPGroup>
                    {OTP_SLOT_IDS.map((slotId, index) => (
                      <InputOTPSlot index={index} key={slotId} />
                    ))}
                  </InputOTPGroup>
                </InputOTP>
                {field.state.meta.errors.length > 0 && (
                  <FieldError className="self-center">
                    {field.state.meta.errors.join(", ")}
                  </FieldError>
                )}
              </Field>
            )}
          </verificationForm.Field>
        </form>
      </div>
    ),
    [
      verificationForm,
      handleVerifyDeletion,
      verificationFormId,
      verificationCodeInputId,
    ]
  );

  const dialogContent = useMemo(
    () => (
      <CredenzaContent className="sm:w-full md:max-w-[600px]">
        <CredenzaHeader>
          <CredenzaTitle>
            {isSingleOrganization
              ? "Delete organization permanently"
              : "Delete this store"}
          </CredenzaTitle>
          <CredenzaDescription>
            {currentStep === "confirmation" && isSingleOrganization && (
              <>
                You are about to permanently delete your only organization. This
                action will completely erase all data associated with your
                account and reset your user status, allowing you to start the
                onboarding process again.
                <br />
                <br />
                This is a destructive and irreversible operation that requires
                email confirmation. Make sure to backup important data before
                proceeding.
              </>
            )}
            {currentStep === "confirmation" && !isSingleOrganization && (
              <>
                Are you sure you want to delete this store? This action is
                irreversible and requires email confirmation.
                <br />
                <br />
                All information related to this store, including products,
                orders, customers, suppliers, employees and settings will be
                permanently deleted and cannot be recovered.
              </>
            )}
            {currentStep === "verification" && (
              <>
                A 6-digit verification code has been sent to your email. Please
                enter the code to confirm the
                {isSingleOrganization
                  ? " permanent deletion of the organization."
                  : " deletion of the store."}
              </>
            )}
          </CredenzaDescription>
        </CredenzaHeader>

        {currentStep === "confirmation" && (
          <>
            {confirmationContent}
            <div className="flex justify-end gap-3">{getFooterJSX}</div>
          </>
        )}

        {currentStep === "verification" && (
          <>
            {verificationContent}
            <div className="flex justify-end gap-3">{getFooterJSX}</div>
          </>
        )}
      </CredenzaContent>
    ),
    [
      currentStep,
      isSingleOrganization,
      confirmationContent,
      verificationContent,
      getFooterJSX,
    ]
  );

  return dialogContent;
};

export default DeleteOrganizationDialog;
