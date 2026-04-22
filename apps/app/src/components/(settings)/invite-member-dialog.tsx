import { HugeiconsIcon } from "@hugeicons/react";
import { Button } from "@strait/ui/components/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@strait/ui/components/dialog";
import { Field, FieldError, FieldLabel } from "@strait/ui/components/field";
import { Input } from "@strait/ui/components/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@strait/ui/components/select";
import { toast } from "@strait/ui/components/toast/index";
import { useForm } from "@tanstack/react-form";
import { useState } from "react";
import { z } from "zod/v4";
import { useCreateInvitation } from "@/hooks/auth/use-invitation";
import { formatFieldErrors } from "@/lib/form-errors";
import { LoadingIcon, PlusIcon } from "@/lib/icons";

const inviteSchema = z.object({
  email: z.string().email("Enter a valid email"),
  role: z.enum(["member", "admin", "owner"]),
});

interface InviteMemberDialogProps {
  organizationId: string;
}

const InviteMemberDialog = ({ organizationId }: InviteMemberDialogProps) => {
  const [open, setOpen] = useState(false);
  const createInvitation = useCreateInvitation();

  const form = useForm({
    defaultValues: {
      email: "",
      role: "member" as "member" | "admin" | "owner",
    },
    validators: { onChange: inviteSchema },
    onSubmit: async ({ value }) => {
      try {
        await createInvitation.mutateAsync({
          email: value.email,
          role: value.role,
          organizationId,
        });
        toast.success(`Invitation sent to ${value.email}`);
        form.reset();
        setOpen(false);
      } catch (error) {
        toast.error(
          error instanceof Error ? error.message : "Failed to send invitation."
        );
      }
    },
  });

  return (
    <Dialog onOpenChange={setOpen} open={open}>
      <DialogTrigger render={<Button />}>
        <HugeiconsIcon className="size-4" icon={PlusIcon} />
        Invite Member
      </DialogTrigger>
      <DialogContent>
        <form
          onSubmit={(e) => {
            e.preventDefault();
            form.handleSubmit();
          }}
        >
          <DialogHeader>
            <DialogTitle>Invite a team member</DialogTitle>
            <DialogDescription>
              Send an invitation to join your organization.
            </DialogDescription>
          </DialogHeader>

          <div className="flex flex-col gap-4 py-4">
            <form.Field name="email">
              {(field) => (
                <Field>
                  <FieldLabel htmlFor={field.name}>Email</FieldLabel>
                  <Input
                    id={field.name}
                    onBlur={field.handleBlur}
                    onChange={(e) => field.handleChange(e.target.value)}
                    placeholder="colleague@example.com"
                    type="email"
                    value={field.state.value}
                  />
                  {field.state.meta.errors.length > 0 && (
                    <FieldError>
                      {formatFieldErrors(field.state.meta.errors)}
                    </FieldError>
                  )}
                </Field>
              )}
            </form.Field>

            <form.Field name="role">
              {(field) => (
                <Field>
                  <FieldLabel>Role</FieldLabel>
                  <Select
                    onValueChange={(val) =>
                      field.handleChange(val as "member" | "admin" | "owner")
                    }
                    value={field.state.value}
                  >
                    <SelectTrigger>
                      <SelectValue placeholder="Select a role" />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="member">Member</SelectItem>
                      <SelectItem value="admin">Admin</SelectItem>
                      <SelectItem value="owner">Owner</SelectItem>
                    </SelectContent>
                  </Select>
                  {field.state.meta.errors.length > 0 && (
                    <FieldError>
                      {formatFieldErrors(field.state.meta.errors)}
                    </FieldError>
                  )}
                </Field>
              )}
            </form.Field>
          </div>

          <DialogFooter>
            <Button
              onClick={() => setOpen(false)}
              type="button"
              variant="outline"
            >
              Cancel
            </Button>
            <form.Subscribe
              selector={(state) => ({
                canSubmit: state.canSubmit,
                isSubmitting: state.isSubmitting,
              })}
            >
              {({ canSubmit, isSubmitting }) => (
                <Button
                  disabled={
                    !canSubmit || isSubmitting || createInvitation.isPending
                  }
                  type="submit"
                >
                  {isSubmitting || createInvitation.isPending ? (
                    <HugeiconsIcon
                      className="size-4 animate-spin"
                      icon={LoadingIcon}
                    />
                  ) : (
                    <HugeiconsIcon className="size-4" icon={PlusIcon} />
                  )}
                  Send Invitation
                </Button>
              )}
            </form.Subscribe>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
};

export default InviteMemberDialog;
