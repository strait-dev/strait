import { standardSchemaResolver } from "@hookform/resolvers/standard-schema";
import { HelpCircleIcon, Loading03Icon } from "@hugeicons/core-free-icons";
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
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from "@strait/ui/components/form";
import { Input } from "@strait/ui/components/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@strait/ui/components/select";
import { Textarea } from "@strait/ui/components/textarea";
import { toast } from "@strait/ui/components/toast/index";
import { createServerFn } from "@tanstack/react-start";
import { format } from "date-fns";
import { useEffect, useId, useState, useTransition } from "react";
import { useForm } from "react-hook-form";
import type z from "zod/v4";
import { resend } from "@/lib/resend";
import { SupportFormSchema } from "@/lib/schema";
import { authMiddleware } from "@/middlewares/auth";
import type { AuthUser } from "@/routes/__root";
import { MILLISECONDS_PER_SECOND, TIMER_INTERVAL_MS } from "@/utils/constants";

const supportAction = createServerFn({ method: "POST" })
  .inputValidator(SupportFormSchema)
  .middleware([authMiddleware])
  .handler(
    async ({ data, context }) =>
      await resend.emails.send({
        from: "Support <hello@usestrait.com>",
        to: "leo@usestrait.com",
        subject: `Support — ${data.email}`,
        react: Support({
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

const STORAGE_KEY = "support_cooldown";

const SupportDialog = ({ user }: Props) => {
  const subjectSelectId = useId();
  const prioritySelectId = useId();
  const environmentSelectId = useId();

  const [open, setOpen] = useState<boolean>(false);
  const [isPending, startTransition] = useTransition();
  const [cooldownTime, setCooldownTime] = useState(0);

  useEffect(() => {
    const storedCooldownEnd = localStorage.getItem(STORAGE_KEY);
    let initialTime = 0;
    if (storedCooldownEnd) {
      const remainingTime = Math.ceil(
        (Number.parseInt(storedCooldownEnd, 10) - Date.now()) /
          MILLISECONDS_PER_SECOND
      );
      if (remainingTime > 0) {
        initialTime = remainingTime;
      } else {
        localStorage.removeItem(STORAGE_KEY);
      }
    }

    if (initialTime > 0) {
      setCooldownTime(initialTime);
      const timer = setInterval(() => {
        setCooldownTime((time) => {
          if (time <= 1) {
            localStorage.removeItem(STORAGE_KEY);
            clearInterval(timer);
            return 0;
          }
          return time - 1;
        });
      }, TIMER_INTERVAL_MS);
      return () => clearInterval(timer);
    }
  }, []);

  const form = useForm<
    z.input<typeof SupportFormSchema>,
    z.output<typeof SupportFormSchema>
  >({
    defaultValues: {
      email: user.email,
      subject: undefined,
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
      toast.promise(supportAction({ data: values }), {
        loading: "Sending request...",
        success: "Request sent successfully",
        error: "Error sending request",
      });
    });
  };

  return (
    <Credenza onOpenChange={setOpen} open={open}>
      <CredenzaTrigger
        render={
          <Button
            aria-label="Get help"
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
          icon={HelpCircleIcon}
        />
      </CredenzaTrigger>

      <CredenzaContent className="flex max-h-[90vh] max-w-3xl flex-col md:max-w-[600px]">
        <CredenzaHeader className="flex-none">
          <CredenzaTitle>Need help?</CredenzaTitle>
          <CredenzaDescription>
            To better assist you, please provide as much information as possible
            about your problem.
          </CredenzaDescription>
        </CredenzaHeader>

        <Form {...form}>
          <form
            className="-mr-4 flex flex-1 flex-col gap-4 overflow-y-auto pr-4"
            onSubmit={form.handleSubmit(onSubmit)}
          >
            <div className="flex flex-1 flex-col gap-4">
              {/* Basic Info Section */}
              <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
                <div className="flex flex-col gap-2">
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

                <div className="flex flex-col gap-2">
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
                            <SelectItem value="technical">
                              Technical Issue
                            </SelectItem>
                            <SelectItem value="billing">Billing</SelectItem>
                            <SelectItem value="account">Account</SelectItem>
                            <SelectItem value="other">Other</SelectItem>
                          </SelectContent>
                        </Select>
                        <FormMessage />
                      </FormItem>
                    )}
                  />
                </div>
              </div>

              {/* Priority and Environment Section */}
              <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
                <div className="flex flex-col gap-2">
                  <FormField
                    control={form.control}
                    name="priority"
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel htmlFor={prioritySelectId}>
                          Priority
                        </FormLabel>
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
                        <FormMessage />
                      </FormItem>
                    )}
                  />
                </div>

                <div className="flex flex-col gap-2">
                  <FormField
                    control={form.control}
                    name="environment"
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel htmlFor={environmentSelectId}>
                          Environment
                        </FormLabel>
                        <Select
                          onValueChange={field.onChange}
                          value={field.value}
                        >
                          <SelectTrigger id={environmentSelectId}>
                            <SelectValue placeholder="Select environment" />
                          </SelectTrigger>
                          <SelectContent>
                            <SelectItem value="production">
                              Production
                            </SelectItem>
                            <SelectItem value="development">
                              Development
                            </SelectItem>
                            <SelectItem value="staging">Staging</SelectItem>
                          </SelectContent>
                        </Select>
                        <FormMessage />
                      </FormItem>
                    )}
                  />
                </div>
              </div>

              {/* Problem Description Section */}
              <div className="space-y-4">
                <div className="flex flex-col gap-2">
                  <FormField
                    control={form.control}
                    name="message"
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>Problem Description</FormLabel>
                        <FormControl>
                          <Textarea
                            className="min-h-[100px] resize-none"
                            placeholder="Describe the problem in detail..."
                            {...field}
                          />
                        </FormControl>
                        <FormMessage />
                      </FormItem>
                    )}
                  />
                </div>

                <div className="flex flex-col gap-2">
                  <FormField
                    control={form.control}
                    name="steps_to_reproduce"
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>Steps to Reproduce</FormLabel>
                        <FormControl>
                          <Textarea
                            className="min-h-[100px] resize-none"
                            placeholder="List the steps needed to reproduce the problem..."
                            {...field}
                          />
                        </FormControl>
                        <FormMessage />
                        <FormDescription>
                          Ex: 1. Accessed page X, 2. Clicked button Y, 3. Filled
                          field Z...
                        </FormDescription>
                      </FormItem>
                    )}
                  />
                </div>
              </div>

              {/* Results Section */}
              <div className="grid grid-cols-1 gap-4">
                <FormField
                  control={form.control}
                  name="expected_result"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>Expected Result</FormLabel>
                      <FormControl>
                        <Textarea
                          className="min-h-[100px] resize-none"
                          placeholder="What should happen?"
                          {...field}
                        />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                <FormField
                  control={form.control}
                  name="actual_result"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>Actual Result</FormLabel>
                      <FormControl>
                        <Textarea
                          className="min-h-[100px] resize-none"
                          placeholder="What is happening?"
                          {...field}
                        />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              </div>
            </div>

            <Button
              className="mt-2 inline-flex w-full flex-none justify-center rounded-custom px-3 py-2 font-semibold"
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
              Send Request {cooldownTime > 0 ? `(${cooldownTime}s)` : ""}
            </Button>
          </form>
        </Form>
      </CredenzaContent>
    </Credenza>
  );
};

export default SupportDialog;
