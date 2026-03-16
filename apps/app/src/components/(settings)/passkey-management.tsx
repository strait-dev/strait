import { HugeiconsIcon } from "@hugeicons/react";
import { Button } from "@strait/ui/components/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@strait/ui/components/card";
import { toast } from "@strait/ui/components/toast/index";
import { useQuery } from "@tanstack/react-query";
import {
  passkeysQueryOptions,
  useAddPasskey,
  useDeletePasskey,
} from "@/hooks/auth/use-account";
import { KeyIcon, LoadingIcon, TrashIcon } from "@/lib/icons";

const PasskeyManagement = () => {
  const { data: passkeys = [], isLoading } = useQuery(passkeysQueryOptions());
  const addPasskey = useAddPasskey();
  const deletePasskey = useDeletePasskey();

  const handleAdd = async () => {
    try {
      await addPasskey.mutateAsync();
      toast.success("Passkey added successfully.");
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : "Failed to add passkey."
      );
    }
  };

  const handleDelete = async (id: string) => {
    try {
      await deletePasskey.mutateAsync(id);
      toast.success("Passkey removed.");
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : "Failed to remove passkey."
      );
    }
  };

  const formatDate = (date: string | Date | null) => {
    if (!date) {
      return "Unknown";
    }
    return new Date(date).toLocaleDateString("en-US", {
      year: "numeric",
      month: "short",
      day: "numeric",
    });
  };

  return (
    <Card>
      <CardHeader>
        <div className="flex items-center justify-between">
          <div>
            <CardTitle>Passkeys</CardTitle>
            <CardDescription>
              Manage passkeys for passwordless sign in.
            </CardDescription>
          </div>
          <Button disabled={addPasskey.isPending} onClick={handleAdd} size="sm">
            {addPasskey.isPending ? (
              <HugeiconsIcon
                className="size-3 animate-spin"
                icon={LoadingIcon}
              />
            ) : (
              <HugeiconsIcon className="size-3" icon={KeyIcon} />
            )}
            Add passkey
          </Button>
        </div>
      </CardHeader>
      <CardContent>
        {isLoading && (
          <div className="flex items-center gap-2 text-muted-foreground text-sm">
            <HugeiconsIcon className="size-4 animate-spin" icon={LoadingIcon} />
            Loading passkeys...
          </div>
        )}
        {!isLoading && passkeys.length === 0 && (
          <p className="text-muted-foreground text-sm">
            No passkeys registered. Add one to enable passwordless sign in.
          </p>
        )}
        {!isLoading && passkeys.length > 0 && (
          <div className="flex flex-col gap-3">
            {passkeys.map((passkey) => {
              const isDeleting =
                deletePasskey.isPending &&
                deletePasskey.variables === passkey.id;

              return (
                <div
                  className="flex items-center justify-between rounded-md border p-3"
                  key={passkey.id}
                >
                  <div className="flex items-center gap-3">
                    <HugeiconsIcon
                      className="size-4 text-muted-foreground"
                      icon={KeyIcon}
                    />
                    <div>
                      <p className="font-medium text-sm">
                        {passkey.name ?? "Passkey"}
                      </p>
                      <p className="text-muted-foreground text-xs">
                        Added {formatDate(passkey.createdAt)}
                      </p>
                    </div>
                  </div>
                  <Button
                    disabled={isDeleting}
                    onClick={() => handleDelete(passkey.id)}
                    size="sm"
                    variant="destructive"
                  >
                    {isDeleting ? (
                      <HugeiconsIcon
                        className="size-3 animate-spin"
                        icon={LoadingIcon}
                      />
                    ) : (
                      <HugeiconsIcon className="size-3" icon={TrashIcon} />
                    )}
                    Remove
                  </Button>
                </div>
              );
            })}
          </div>
        )}
      </CardContent>
    </Card>
  );
};

export default PasskeyManagement;
