import { standardSchemaResolver } from "@hookform/resolvers/standard-schema";
import { BubbleChatIcon, Loading03Icon } from "@hugeicons/core-free-icons";
import { HugeiconsIcon } from "@hugeicons/react";
import { resend } from "@strait/mail/index.ts";
import { Feedback } from "@strait/transactional";
import { Button } from "@strait/ui/components/button.tsx";
import {
  Credenza,
  CredenzaContent,
  CredenzaDescription,
  CredenzaHeader,
  CredenzaTitle,
  CredenzaTrigger,
} from "@strait/ui/components/credenza.tsx";
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from "@strait/ui/components/form.tsx";
import { Input } from "@strait/ui/components/input.tsx";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@strait/ui/components/select.tsx";
import { Textarea } from "@strait/ui/components/textarea.tsx";
import { toast } from "@strait/ui/components/toast/index.ts";
import { createServerFn } from "@tanstack/react-start";
import { format } from "date-fns";
import { useEffect, useId, useState, useTransition } from "react";
import { useForm } from "react-hook-form";
import type z from "zod/v4";
import { FeedbackFormSchema } from "@/lib/schema.ts";
import { authMiddleware } from "@/middlewares/auth.ts";
import type { AuthUser } from "@/routes/__root.tsx";
import {
  MILLISECONDS_PER_SECOND,
  TIMER_INTERVAL_MS,
} from "@/utils/constants.ts";

const feedbackAction = createServerFn({ method: "POST" })
  .inputValidator(FeedbackFormSchema)
  .middleware([authMiddleware])
  .handler(
    async ({ data, context }) =>
      await resend.emails.send({
        from: "Feedback <hello@usestrait.com>",
        to: "leo@usestrait.com",
        subject: `Feedback — ${data.email}`,
        react: Feedback({
          ...data,
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
      })
  );

type Props = {
  user: AuthUser;
};

const STORAGE_KEY = "feedback_cooldown";

const FeedbackDialog = ({ user }: Props) => {
  // Generate unique ID for form element
  const subjectSelectId = useId();

  const [open, setOpen] = useState<boolean>(false);
  const [isPending, startTransition] = useTransition();
  const [cooldownTime, setCooldownTime] = useState(0);

  useEffect(() => {
    const storedCooldownEnd = localStorage.getItem(STORAGE_KEY);
    if (storedCooldownEnd) {
      const remainingTime = Math.ceil(
        (Number.parseInt(storedCooldownEnd, 10) - Date.now()) /
          MILLISECONDS_PER_SECOND
      );
      if (remainingTime > 0) {
        setCooldownTime(remainingTime);
      } else {
        localStorage.removeItem(STORAGE_KEY);
      }
    }
  }, []);

  const isCoolingDown = cooldownTime > 0;
  useEffect(() => {
    if (!isCoolingDown) {
      return;
    }

    const timer = setInterval(() => {
      setCooldownTime((time) => {
        if (time <= 1) {
          localStorage.removeItem(STORAGE_KEY);
          return 0;
        }
        return time - 1;
      });
    }, TIMER_INTERVAL_MS);
    return () => clearInterval(timer);
  }, [isCoolingDown]);

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
      toast.promise(feedbackAction({ data: values }), {
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
            className="text-muted-foreground/65 group-data-[active=true]/menu-button:text-primary"
            disabled={cooldownTime > 0}
            size="icon"
            variant="outline"
          />
        }
      >
        <HugeiconsIcon
          aria-hidden="true"
          className="size-4"
          icon={BubbleChatIcon}
        />
      </CredenzaTrigger>

      <CredenzaContent>
        <CredenzaHeader>
          <CredenzaTitle>Do you have any suggestions?</CredenzaTitle>
          <CredenzaDescription>
            Send your feedback to help us improve Strait. Your feedback is very
            important to us.
          </CredenzaDescription>
        </CredenzaHeader>

        <Form {...form}>
          <form
            className="flex w-full flex-col gap-1"
            onSubmit={form.handleSubmit(onSubmit)}
          >
            <div className="flex w-full flex-col items-center space-y-5">
              <div className="flex w-full flex-col gap-2">
                <FormField
                  control={form.control}
                  name="email"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>Email</FormLabel>
                      <FormControl>
                        <Input
                          {...field}
                          disabled={true}
                          placeholder="Enter your email"
                          value={field.value}
                        />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              </div>

              <div className="flex w-full flex-col gap-2">
                <FormField
                  control={form.control}
                  name="subject"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel htmlFor={subjectSelectId}>Subject</FormLabel>
                      <Select
                        onValueChange={field.onChange}
                        value={field.value}
                      >
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
                      <FormMessage />
                    </FormItem>
                  )}
                />
              </div>

              <div className="flex w-full flex-col gap-2">
                <FormField
                  control={form.control}
                  name="message"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>Message</FormLabel>
                      <FormControl>
                        <Textarea
                          className="resize-none"
                          placeholder="Enter your message..."
                          {...field}
                        />
                      </FormControl>
                      <FormMessage />
                      <FormDescription>
                        The message must have at least 10 characters.
                      </FormDescription>
                    </FormItem>
                  )}
                />
              </div>

              <Button
                className="inline-flex w-full justify-center rounded-custom px-3 py-2 font-semibold"
                disabled={
                  form.formState.isSubmitting || isPending || cooldownTime > 0
                }
                type="submit"
              >
                {form.formState.isSubmitting || isPending ? (
                  <HugeiconsIcon
                    className="size-4 animate-spin"
                    icon={Loading03Icon}
                  />
                ) : null}
                Send feedback {cooldownTime > 0 ? `(${cooldownTime}s)` : ""}
              </Button>
            </div>
          </form>
        </Form>
      </CredenzaContent>
    </Credenza>
  );
};

export default FeedbackDialog;
