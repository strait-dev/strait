import { standardSchemaResolver } from "@hookform/resolvers/standard-schema";
import { HugeiconsIcon } from "@hugeicons/react";
import { Feedback } from "@strait/transactional";
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
import { ChatIcon } from "@/lib/icons";
import { enforceRateLimit } from "@/lib/rate-limit.server";
import { getResend } from "@/lib/resend.server";
import { FeedbackFormSchema } from "@/lib/schema";
import { authMiddleware } from "@/middlewares/auth";
import type { AuthUser } from "@/routes/__root";

const FEEDBACK_COOLDOWN_SECONDS = 60;

const feedbackAction = createServerFn({ method: "POST" })
  .middleware([authMiddleware])
  .inputValidator(FeedbackFormSchema)
  .handler(async ({ data, context }) => {
    await enforceRateLimit({
      key: `feedback:${context.user.id}`,
      limit: 5,
      windowSeconds: 3600,
    });

    const email = context.user.email;
    if (!email) {
      throw new Error("Authenticated email is required");
    }

    return await getResend().emails.send({
      from: "Feedback <hello@usestrait.com>",
      to: "leo@strait.dev",
      subject: `Feedback — ${email}`,
      react: Feedback({
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

const STORAGE_KEY = "feedback_cooldown";

const FeedbackDialog = ({ user }: Props) => {
  // Generate unique ID for form element
  const subjectSelectId = useId();

  const [open, setOpen] = useState<boolean>(false);
  const [isPending, startTransition] = useTransition();
  const cooldownTime = useCooldownTime(STORAGE_KEY);

  const form = useForm<
    z.input<typeof FeedbackFormSchema>,
    z.output<typeof FeedbackFormSchema>
  >({
    defaultValues: {
      email: user.email,
      subject: "",
      message: "",
    },
    mode: "onChange",
    resolver: standardSchemaResolver(FeedbackFormSchema),
  });

  const onSubmit = (values: z.input<typeof FeedbackFormSchema>) => {
    if (cooldownTime > 0) {
      return;
    }

    startTransition(() => {
      const promise = feedbackAction({ data: values }).then((result) => {
        startCooldown(STORAGE_KEY, FEEDBACK_COOLDOWN_SECONDS);
        getPostHog()?.capture("feedback_submitted");
        return result;
      });
      toast.promise(promise, {
        loading: "Sending feedback...",
        success: "Feedback sent successfully",
        error: "Error sending feedback",
      });
    });
  };

  return (
    <Credenza onOpenChange={setOpen} open={open}>
      <CredenzaTrigger
        render={
          <Button
            aria-label="Send feedback"
            className="text-muted-foreground group-data-[active=true]/menu-button:text-primary"
            disabled={cooldownTime > 0}
            size="icon"
            type="button"
            variant="outline"
          >
            <HugeiconsIcon
              aria-hidden="true"
              className="size-4"
              icon={ChatIcon}
            />
            <span className="sr-only">Send feedback</span>
          </Button>
        }
      />

      <CredenzaContent>
        <CredenzaHeader>
          <CredenzaTitle>Do you have any suggestions?</CredenzaTitle>
          <CredenzaDescription>
            Send your feedback to help us improve Strait. Your feedback is very
            important to us.
          </CredenzaDescription>
        </CredenzaHeader>

        <form
          className="flex w-full flex-col gap-1"
          onSubmit={form.handleSubmit(onSubmit)}
        >
          <div className="flex w-full flex-col items-center space-y-5">
            <div className="flex w-full flex-col gap-2">
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

            <div className="flex w-full flex-col gap-2">
              <Controller
                control={form.control}
                name="subject"
                render={({ field, fieldState }) => (
                  <Field data-invalid={fieldState.invalid}>
                    <FieldLabel htmlFor={subjectSelectId}>Subject</FieldLabel>
                    <Select onValueChange={field.onChange} value={field.value}>
                      <SelectTrigger id={subjectSelectId}>
                        <SelectValue placeholder="Select a subject" />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="bug">Bug</SelectItem>
                        <SelectItem value="feedback">Feedback</SelectItem>
                        <SelectItem value="featureRequest">
                          Feature request
                        </SelectItem>
                        <SelectItem value="question">Question</SelectItem>
                        <SelectItem value="other">Other</SelectItem>
                      </SelectContent>
                    </Select>
                    <FieldError>{fieldState.error?.message}</FieldError>
                  </Field>
                )}
              />
            </div>

            <div className="flex w-full flex-col gap-2">
              <Controller
                control={form.control}
                name="message"
                render={({ field, fieldState }) => (
                  <Field data-invalid={fieldState.invalid}>
                    <FieldLabel>Message</FieldLabel>
                    <Textarea
                      className="resize-none"
                      placeholder="Enter your message..."
                      {...field}
                    />
                    <FieldError>{fieldState.error?.message}</FieldError>
                    <FieldDescription>
                      The message must have at least 10 characters.
                    </FieldDescription>
                  </Field>
                )}
              />
            </div>

            <Button
              className="w-full"
              disabled={
                form.formState.isSubmitting || isPending || cooldownTime > 0
              }
              type="submit"
            >
              {form.formState.isSubmitting || isPending ? <Spinner /> : null}
              Send feedback {cooldownTime > 0 ? `(${cooldownTime}s)` : ""}
            </Button>
          </div>
        </form>
      </CredenzaContent>
    </Credenza>
  );
};

export default FeedbackDialog;
