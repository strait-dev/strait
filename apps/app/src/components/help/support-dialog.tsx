import { standardSchemaResolver } from "@hookform/resolvers/standard-schema";
import { HugeiconsIcon } from "@hugeicons/react";
import { Support } from "@strait/transactional";
import { Button } from "@strait/ui/components/button";
import {
  Credenza,
  CredenzaContent,
  CredenzaDescription,
  CredenzaHeader,
  CredenzaTitle,
  CredenzaTrigger,
} from "@strait/ui/components/credenza";
import {
  Field,
  FieldDescription,
  FieldError,
  FieldLabel,
} from "@strait/ui/components/field";
import { Input } from "@strait/ui/components/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@strait/ui/components/select";
import { Spinner } from "@strait/ui/components/spinner";
import { Textarea } from "@strait/ui/components/textarea";
import { toast } from "@strait/ui/components/toast";
import { createServerFn } from "@tanstack/react-start";
import { format } from "date-fns";
import { useId, useState, useTransition } from "react";
import { Controller, useForm } from "react-hook-form";
import type z from "zod/v4";
import { startCooldown, useCooldownTime } from "@/hooks/use-cooldown-time";
import { getPostHog } from "@/lib/analytics";
import { HelpCircleIcon } from "@/lib/icons";
import { enforceRateLimit } from "@/lib/rate-limit.server";
import { getResend } from "@/lib/resend.server";
import { SupportFormSchema } from "@/lib/schema";
import { authMiddleware } from "@/middlewares/auth";
import type { AuthUser } from "@/routes/__root";

const SUPPORT_COOLDOWN_SECONDS = 60;

const supportAction = createServerFn({ method: "POST" })
  .middleware([authMiddleware])
  .inputValidator(SupportFormSchema)
  .handler(async ({ data, context }) => {
    await enforceRateLimit({
      key: `support:${context.user.id}`,
      limit: 5,
      windowSeconds: 3600,
    });

    const email = context.user.email;
    if (!email) {
      throw new Error("Authenticated email is required");
    }

    return await getResend().emails.send({
      from: "Support <hello@usestrait.com>",
      to: "leo@strait.dev",
      subject: `Support — ${email}`,
      react: Support({
        ...data,
        email,
        name: context.user.name,
        date: format(new Date(), "MMMM dd, yyyy"),
        createdAt: format(
          new Date(context.user.createdAt as unknown as string),
          "MMMM dd, yyyy"
        ),
        lastLogin: format(
          new Date(context.user.updatedAt as unknown as string),
          "MMMM dd, yyyy"
        ),
      }),
    });
  });

type Props = {
  user: AuthUser;
};

const STORAGE_KEY = "support_cooldown";

const SupportDialog = ({ user }: Props) => {
  const subjectSelectId = useId();
  const prioritySelectId = useId();
  const environmentSelectId = useId();

  const [open, setOpen] = useState<boolean>(false);
  const [isPending, startTransition] = useTransition();
  const cooldownTime = useCooldownTime(STORAGE_KEY);

  const form = useForm<
    z.input<typeof SupportFormSchema>,
    z.output<typeof SupportFormSchema>
  >({
    defaultValues: {
      email: user.email,
      subject: "",
      priority: "low",
      environment: "production",
      message: "",
      steps_to_reproduce: "",
      expected_result: "",
      actual_result: "",
    },
    mode: "onChange",
    resolver: standardSchemaResolver(SupportFormSchema),
  });

  const onSubmit = (values: z.input<typeof SupportFormSchema>) => {
    if (cooldownTime > 0) {
      return;
    }

    startTransition(() => {
      const promise = supportAction({ data: values }).then((result) => {
        startCooldown(STORAGE_KEY, SUPPORT_COOLDOWN_SECONDS);
        getPostHog()?.capture("support_submitted");
        return result;
      });
      toast.promise(promise, {
        loading: "Sending request...",
        success: "Request sent successfully",
        error: "Error sending request",
      });
    });
  };

  return (
    <Credenza
      onOpenChange={(isOpen) => {
        setOpen(isOpen);
        if (isOpen) {
          getPostHog()?.capture("support_opened");
        }
      }}
      open={open}
    >
      <CredenzaTrigger
        render={
          <Button
            aria-label="Get help"
            className="text-muted-foreground group-data-[active=true]/menu-button:text-primary"
            disabled={cooldownTime > 0}
            size="icon"
            type="button"
            variant="outline"
          >
            <HugeiconsIcon
              aria-hidden="true"
              className="size-4"
              icon={HelpCircleIcon}
            />
            <span className="sr-only">Get help</span>
          </Button>
        }
      />

      <CredenzaContent className="flex max-h-[90vh] max-w-3xl flex-col md:max-w-[600px]">
        <CredenzaHeader className="flex-none">
          <CredenzaTitle>Need help?</CredenzaTitle>
          <CredenzaDescription>
            To better assist you, please provide as much information as possible
            about your problem.
          </CredenzaDescription>
        </CredenzaHeader>

        <form
          className="-mr-4 flex flex-1 flex-col gap-4 overflow-y-auto pr-4"
          onSubmit={form.handleSubmit(onSubmit)}
        >
          <div className="flex flex-1 flex-col gap-4">
            <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
              <div className="flex flex-col gap-2">
                <Controller
                  control={form.control}
                  name="email"
                  render={({ field, fieldState }) => (
                    <Field data-invalid={fieldState.invalid}>
                      <FieldLabel>Email</FieldLabel>
                      <Input
                        {...field}
                        disabled={true}
                        placeholder="Enter your email"
                        value={field.value}
                      />
                      <FieldError>{fieldState.error?.message}</FieldError>
                    </Field>
                  )}
                />
              </div>

              <div className="flex flex-col gap-2">
                <Controller
                  control={form.control}
                  name="subject"
                  render={({ field, fieldState }) => (
                    <Field data-invalid={fieldState.invalid}>
                      <FieldLabel htmlFor={subjectSelectId}>Subject</FieldLabel>
                      <Select
                        onValueChange={field.onChange}
                        value={field.value}
                      >
                        <SelectTrigger id={subjectSelectId}>
                          <SelectValue placeholder="Select a subject" />
                        </SelectTrigger>
                        <SelectContent>
                          <SelectItem value="technical">
                            Technical issue
                          </SelectItem>
                          <SelectItem value="billing">Billing</SelectItem>
                          <SelectItem value="account">Account</SelectItem>
                          <SelectItem value="other">Other</SelectItem>
                        </SelectContent>
                      </Select>
                      <FieldError>{fieldState.error?.message}</FieldError>
                    </Field>
                  )}
                />
              </div>
            </div>

            <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
              <div className="flex flex-col gap-2">
                <Controller
                  control={form.control}
                  name="priority"
                  render={({ field, fieldState }) => (
                    <Field data-invalid={fieldState.invalid}>
                      <FieldLabel htmlFor={prioritySelectId}>
                        Priority
                      </FieldLabel>
                      <Select
                        onValueChange={field.onChange}
                        value={field.value}
                      >
                        <SelectTrigger id={prioritySelectId}>
                          <SelectValue placeholder="Select priority" />
                        </SelectTrigger>
                        <SelectContent>
                          <SelectItem value="low">Low</SelectItem>
                          <SelectItem value="medium">Medium</SelectItem>
                          <SelectItem value="high">High</SelectItem>
                        </SelectContent>
                      </Select>
                      <FieldError>{fieldState.error?.message}</FieldError>
                    </Field>
                  )}
                />
              </div>

              <div className="flex flex-col gap-2">
                <Controller
                  control={form.control}
                  name="environment"
                  render={({ field, fieldState }) => (
                    <Field data-invalid={fieldState.invalid}>
                      <FieldLabel htmlFor={environmentSelectId}>
                        Environment
                      </FieldLabel>
                      <Select
                        onValueChange={field.onChange}
                        value={field.value}
                      >
                        <SelectTrigger id={environmentSelectId}>
                          <SelectValue placeholder="Select environment" />
                        </SelectTrigger>
                        <SelectContent>
                          <SelectItem value="production">Production</SelectItem>
                          <SelectItem value="development">
                            Development
                          </SelectItem>
                          <SelectItem value="staging">Staging</SelectItem>
                        </SelectContent>
                      </Select>
                      <FieldError>{fieldState.error?.message}</FieldError>
                    </Field>
                  )}
                />
              </div>
            </div>

            <div className="space-y-4">
              <div className="flex flex-col gap-2">
                <Controller
                  control={form.control}
                  name="message"
                  render={({ field, fieldState }) => (
                    <Field data-invalid={fieldState.invalid}>
                      <FieldLabel>Problem description</FieldLabel>
                      <Textarea
                        className="min-h-[100px] resize-none"
                        placeholder="Describe the problem in detail..."
                        {...field}
                      />
                      <FieldError>{fieldState.error?.message}</FieldError>
                    </Field>
                  )}
                />
              </div>

              <div className="flex flex-col gap-2">
                <Controller
                  control={form.control}
                  name="steps_to_reproduce"
                  render={({ field, fieldState }) => (
                    <Field data-invalid={fieldState.invalid}>
                      <FieldLabel>Steps to Reproduce</FieldLabel>
                      <Textarea
                        className="min-h-[100px] resize-none"
                        placeholder="List the steps needed to reproduce the problem..."
                        {...field}
                      />
                      <FieldError>{fieldState.error?.message}</FieldError>
                      <FieldDescription>
                        Ex: 1. Accessed page X, 2. Clicked button Y, 3. Filled
                        field Z...
                      </FieldDescription>
                    </Field>
                  )}
                />
              </div>
            </div>

            <div className="grid grid-cols-1 gap-4">
              <Controller
                control={form.control}
                name="expected_result"
                render={({ field, fieldState }) => (
                  <Field data-invalid={fieldState.invalid}>
                    <FieldLabel>Expected result</FieldLabel>
                    <Textarea
                      className="min-h-[100px] resize-none"
                      placeholder="What should happen?"
                      {...field}
                    />
                    <FieldError>{fieldState.error?.message}</FieldError>
                  </Field>
                )}
              />

              <Controller
                control={form.control}
                name="actual_result"
                render={({ field, fieldState }) => (
                  <Field data-invalid={fieldState.invalid}>
                    <FieldLabel>Actual result</FieldLabel>
                    <Textarea
                      className="min-h-[100px] resize-none"
                      placeholder="What is happening?"
                      {...field}
                    />
                    <FieldError>{fieldState.error?.message}</FieldError>
                  </Field>
                )}
              />
            </div>
          </div>

          <Button
            className="mt-2 w-full"
            disabled={
              form.formState.isSubmitting || isPending || cooldownTime > 0
            }
            type="submit"
          >
            {form.formState.isSubmitting || isPending ? <Spinner /> : null}
            Send request {cooldownTime > 0 ? `(${cooldownTime}s)` : ""}
          </Button>
        </form>
      </CredenzaContent>
    </Credenza>
  );
};

export default SupportDialog;
