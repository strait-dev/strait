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
  AlertDialogTrigger,
} from "@strait/ui/components/alert-dialog";
import { Badge } from "@strait/ui/components/badge";
import { Button } from "@strait/ui/components/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@strait/ui/components/card";
import { Checkbox } from "@strait/ui/components/checkbox";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@strait/ui/components/dialog";
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyTitle,
} from "@strait/ui/components/empty";
import { Field, FieldError, FieldLabel } from "@strait/ui/components/field";
import { Input } from "@strait/ui/components/input";
import { SecretInput } from "@strait/ui/components/secret-input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@strait/ui/components/select";
import { Spinner } from "@strait/ui/components/spinner";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@strait/ui/components/table";
import { toast } from "@strait/ui/components/toast";
import { useForm } from "@tanstack/react-form";
import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { z } from "zod/v4";
import {
  apiKeysQueryOptions,
  useCreateApiKey,
  useRevokeApiKey,
} from "@/hooks/api/use-api-keys";
import { formatFieldErrors } from "@/lib/form-errors";
import { PlusIcon, TrashIcon } from "@/lib/icons";

const AVAILABLE_SCOPES = [
  "jobs:read",
  "jobs:write",
  "jobs:trigger",
  "runs:read",
  "workflows:read",
  "workflows:write",
  "webhooks:read",
  "webhooks:write",
  "api-keys:manage",
] as const;

const EXPIRATION_OPTIONS = [
  { label: "Never", value: "" },
  { label: "30 days", value: "30" },
  { label: "60 days", value: "60" },
  { label: "90 days", value: "90" },
  { label: "1 year", value: "365" },
] as const;

const createKeySchema = z.object({
  name: z.string().min(1, "Key name is required"),
  scopes: z.array(z.string()).min(1, "Select at least one scope"),
  expiresInDays: z.string(),
});

const formatDate = (iso: string | null) => {
  if (!iso) {
    return "Never";
  }
  return new Date(iso).toLocaleDateString("en-US", {
    year: "numeric",
    month: "short",
    day: "numeric",
  });
};

const ApiKeysManagement = () => {
  const { data, isLoading } = useQuery(apiKeysQueryOptions());
  const createKey = useCreateApiKey();
  const revokeKey = useRevokeApiKey();
  const [createdKey, setCreatedKey] = useState<string | null>(null);
  const [createOpen, setCreateOpen] = useState(false);

  const keys = data?.data ?? [];

  const form = useForm({
    defaultValues: {
      name: "",
      scopes: ["jobs:read", "jobs:write", "jobs:trigger"] as string[],
      expiresInDays: "",
    },
    validators: { onChange: createKeySchema },
    onSubmit: async ({ value }) => {
      try {
        const expiresInDays = value.expiresInDays
          ? Number(value.expiresInDays)
          : undefined;
        const result = await createKey.mutateAsync({
          name: value.name,
          scopes: value.scopes,
          expiresInDays,
        });
        setCreatedKey(result.key);
        toast.success(`API key "${value.name}" created.`);
        form.reset();
      } catch (error) {
        toast.error(
          error instanceof Error ? error.message : "Failed to create API key."
        );
      }
    },
  });

  const handleRevoke = async (keyId: string, keyName: string) => {
    try {
      await revokeKey.mutateAsync(keyId);
      toast.success(`API key "${keyName}" revoked.`);
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : "Failed to revoke API key."
      );
    }
  };

  return (
    <div className="space-y-6">
      <Card>
        <CardHeader>
          <div className="flex items-center justify-between">
            <div>
              <CardTitle>API Keys</CardTitle>
              <CardDescription>
                Manage API keys for programmatic access to your organization.
              </CardDescription>
            </div>
            <Dialog
              onOpenChange={(open) => {
                setCreateOpen(open);
                if (!open) {
                  setCreatedKey(null);
                  form.reset();
                }
              }}
              open={createOpen}
            >
              <DialogTrigger render={<Button />}>
                <HugeiconsIcon className="size-4" icon={PlusIcon} />
                Create Key
              </DialogTrigger>
              <DialogContent>
                {createdKey ? (
                  <>
                    <DialogHeader>
                      <DialogTitle>API Key Created</DialogTitle>
                      <DialogDescription>
                        Copy this key now. You won't be able to see it again.
                      </DialogDescription>
                    </DialogHeader>
                    <div className="py-4">
                      <SecretInput
                        aria-label="Created API key"
                        className="font-mono"
                        readOnly
                        value={createdKey}
                      />
                    </div>
                    <DialogFooter>
                      <Button onClick={() => setCreateOpen(false)}>Done</Button>
                    </DialogFooter>
                  </>
                ) : (
                  <form
                    onSubmit={(e) => {
                      e.preventDefault();
                      form.handleSubmit();
                    }}
                  >
                    <DialogHeader>
                      <DialogTitle>Create API Key</DialogTitle>
                      <DialogDescription>
                        Create a new API key for programmatic access.
                      </DialogDescription>
                    </DialogHeader>
                    <div className="flex flex-col gap-4 py-4">
                      <form.Field name="name">
                        {(field) => (
                          <Field>
                            <FieldLabel htmlFor={field.name}>
                              Key Name
                            </FieldLabel>
                            <Input
                              aria-describedby={
                                field.state.meta.isTouched &&
                                field.state.meta.errors.length > 0
                                  ? `${field.name}-error`
                                  : undefined
                              }
                              aria-invalid={
                                field.state.meta.isTouched &&
                                field.state.meta.errors.length > 0
                              }
                              id={field.name}
                              onBlur={field.handleBlur}
                              onChange={(e) =>
                                field.handleChange(e.target.value)
                              }
                              placeholder="e.g. Production API Key"
                              type="text"
                              value={field.state.value}
                            />
                            {field.state.meta.isTouched &&
                              field.state.meta.errors.length > 0 && (
                                <FieldError id={`${field.name}-error`}>
                                  {formatFieldErrors(field.state.meta.errors)}
                                </FieldError>
                              )}
                          </Field>
                        )}
                      </form.Field>

                      <form.Field name="scopes">
                        {(field) => (
                          <Field>
                            <FieldLabel>Scopes</FieldLabel>
                            <div className="grid grid-cols-2 gap-2">
                              {AVAILABLE_SCOPES.map((scope) => (
                                <div
                                  className="flex items-center gap-2"
                                  key={scope}
                                >
                                  <Checkbox
                                    checked={field.state.value.includes(scope)}
                                    id={`scope-${scope}`}
                                    onCheckedChange={(checked) => {
                                      const current = field.state.value;
                                      if (checked) {
                                        field.handleChange([...current, scope]);
                                      } else {
                                        field.handleChange(
                                          current.filter((s) => s !== scope)
                                        );
                                      }
                                    }}
                                  />
                                  <label
                                    className="cursor-pointer text-sm"
                                    htmlFor={`scope-${scope}`}
                                  >
                                    {scope}
                                  </label>
                                </div>
                              ))}
                            </div>
                            {field.state.meta.isTouched &&
                              field.state.meta.errors.length > 0 && (
                                <FieldError id={`${field.name}-error`}>
                                  {formatFieldErrors(field.state.meta.errors)}
                                </FieldError>
                              )}
                          </Field>
                        )}
                      </form.Field>

                      <form.Field name="expiresInDays">
                        {(field) => (
                          <Field>
                            <FieldLabel htmlFor={field.name}>
                              Expiration
                            </FieldLabel>
                            <Select
                              onValueChange={(value) =>
                                field.handleChange(value ?? "")
                              }
                              value={field.state.value}
                            >
                              <SelectTrigger>
                                <SelectValue placeholder="Never" />
                              </SelectTrigger>
                              <SelectContent>
                                {EXPIRATION_OPTIONS.map((option) => (
                                  <SelectItem
                                    key={option.value}
                                    value={option.value}
                                  >
                                    {option.label}
                                  </SelectItem>
                                ))}
                              </SelectContent>
                            </Select>
                          </Field>
                        )}
                      </form.Field>
                    </div>
                    <DialogFooter>
                      <Button
                        onClick={() => setCreateOpen(false)}
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
                              !canSubmit || isSubmitting || createKey.isPending
                            }
                            type="submit"
                          >
                            {isSubmitting || createKey.isPending ? (
                              <Spinner />
                            ) : (
                              <HugeiconsIcon
                                className="size-4"
                                icon={PlusIcon}
                              />
                            )}
                            Create Key
                          </Button>
                        )}
                      </form.Subscribe>
                    </DialogFooter>
                  </form>
                )}
              </DialogContent>
            </Dialog>
          </div>
        </CardHeader>
        <CardContent>
          {isLoading && (
            <div className="flex items-center gap-2 text-muted-foreground text-sm">
              <Spinner />
              Loading API keys...
            </div>
          )}
          {!isLoading && keys.length === 0 && (
            <Empty border={false} className="py-4">
              <EmptyHeader>
                <EmptyTitle>No API keys created yet</EmptyTitle>
                <EmptyDescription>
                  Create an API key to authenticate requests from your own
                  services.
                </EmptyDescription>
              </EmptyHeader>
            </Empty>
          )}
          {!isLoading && keys.length > 0 && (
            <Table size="lg" variant="bordered">
              <TableHeader>
                <TableRow>
                  <TableHead scope="col">Name</TableHead>
                  <TableHead scope="col">Key</TableHead>
                  <TableHead className="hidden md:table-cell" scope="col">
                    Scopes
                  </TableHead>
                  <TableHead className="hidden sm:table-cell" scope="col">
                    Created
                  </TableHead>
                  <TableHead className="hidden sm:table-cell" scope="col">
                    Last Used
                  </TableHead>
                  <TableHead className="text-right" scope="col" />
                </TableRow>
              </TableHeader>
              <TableBody>
                {keys.map((key) => {
                  const isRevoking =
                    revokeKey.isPending && revokeKey.variables === key.id;
                  return (
                    <TableRow key={key.id}>
                      <TableCell className="font-medium">{key.name}</TableCell>
                      <TableCell>
                        <Badge className="font-mono" variant="secondary">
                          {key.key_prefix}...
                        </Badge>
                      </TableCell>
                      <TableCell className="hidden md:table-cell">
                        <div className="flex flex-wrap gap-1">
                          {(key.scopes ?? []).map((scope) => (
                            <Badge key={scope} variant="outline">
                              {scope}
                            </Badge>
                          ))}
                        </div>
                      </TableCell>
                      <TableCell className="hidden text-muted-foreground sm:table-cell">
                        {formatDate(key.created_at)}
                      </TableCell>
                      <TableCell className="hidden text-muted-foreground sm:table-cell">
                        {formatDate(
                          (key as Record<string, unknown>).last_used_at as
                            | string
                            | null
                        )}
                      </TableCell>
                      <TableCell className="text-right">
                        <AlertDialog>
                          <AlertDialogTrigger
                            render={
                              <Button
                                disabled={isRevoking}
                                variant="destructive"
                              />
                            }
                          >
                            {isRevoking ? (
                              <Spinner size="xs" />
                            ) : (
                              <HugeiconsIcon
                                className="size-3"
                                icon={TrashIcon}
                              />
                            )}
                            Revoke
                          </AlertDialogTrigger>
                          <AlertDialogContent>
                            <AlertDialogHeader>
                              <AlertDialogTitle>
                                Revoke "{key.name}"?
                              </AlertDialogTitle>
                              <AlertDialogDescription>
                                This will permanently revoke this API key. Any
                                applications using it will lose access
                                immediately.
                              </AlertDialogDescription>
                            </AlertDialogHeader>
                            <AlertDialogFooter>
                              <AlertDialogCancel>Cancel</AlertDialogCancel>
                              <AlertDialogAction
                                onClick={() => handleRevoke(key.id, key.name)}
                              >
                                Revoke Key
                              </AlertDialogAction>
                            </AlertDialogFooter>
                          </AlertDialogContent>
                        </AlertDialog>
                      </TableCell>
                    </TableRow>
                  );
                })}
              </TableBody>
            </Table>
          )}
        </CardContent>
      </Card>
    </div>
  );
};

export default ApiKeysManagement;
