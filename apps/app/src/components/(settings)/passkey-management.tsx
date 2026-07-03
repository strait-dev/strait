import { HugeiconsIcon } from "@hugeicons/react";
import { Button } from "@strait/ui/components/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@strait/ui/components/card";
import {
  Item,
  ItemActions,
  ItemContent,
  ItemDescription,
  ItemGroup,
  ItemMedia,
  ItemTitle,
} from "@strait/ui/components/item";
import { Spinner } from "@strait/ui/components/spinner";
import { toast } from "@strait/ui/components/toast";
import { useQuery } from "@tanstack/react-query";
import {
  passkeysQueryOptions,
  useAddPasskey,
  useDeletePasskey,
} from "@/hooks/auth/use-account";
import { KeyIcon, TrashIcon } from "@/lib/icons";

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
          <Button disabled={addPasskey.isPending} onClick={handleAdd}>
            {addPasskey.isPending ? (
              <Spinner size="xs" />
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
            <Spinner />
            Loading passkeys...
          </div>
        )}
        {!isLoading && passkeys.length === 0 && (
          <p className="text-muted-foreground text-sm">
            No passkeys registered. Add one to enable passwordless sign in.
          </p>
        )}
        {!isLoading && passkeys.length > 0 && (
          <ItemGroup>
            {passkeys.map((passkey) => {
              const isDeleting =
                deletePasskey.isPending &&
                deletePasskey.variables === passkey.id;

              return (
                <Item key={passkey.id} variant="outline">
                  <ItemMedia variant="icon">
                    <HugeiconsIcon
                      className="size-4 text-muted-foreground"
                      icon={KeyIcon}
                    />
                  </ItemMedia>
                  <ItemContent>
                    <ItemTitle>{passkey.name ?? "Passkey"}</ItemTitle>
                    <ItemDescription>
                      Added {formatDate(passkey.createdAt)}
                    </ItemDescription>
                  </ItemContent>
                  <ItemActions>
                    <Button
                      disabled={isDeleting}
                      onClick={() => handleDelete(passkey.id)}
                      variant="destructive"
                    >
                      {isDeleting ? (
                        <Spinner size="xs" />
                      ) : (
                        <HugeiconsIcon className="size-3" icon={TrashIcon} />
                      )}
                      Remove
                    </Button>
                  </ItemActions>
                </Item>
              );
            })}
          </ItemGroup>
        )}
      </CardContent>
    </Card>
  );
};

export default PasskeyManagement;
