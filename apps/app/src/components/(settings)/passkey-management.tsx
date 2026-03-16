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
import { useCallback, useEffect, useState } from "react";
import { authClient } from "@/lib/auth-client";
import { KeyIcon, LoadingIcon, TrashIcon } from "@/lib/icons";
import { captureException } from "@/lib/sentry";

type Passkey = {
  id: string;
  name: string | null;
  createdAt: string | Date | null;
};

const PasskeyManagement = () => {
  const [passkeys, setPasskeys] = useState<Passkey[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [isAdding, setIsAdding] = useState(false);
  const [deletingId, setDeletingId] = useState<string | null>(null);

  const fetchPasskeys = useCallback(async () => {
    try {
      const result = await authClient.passkey.listUserPasskeys();
      if (result.data) {
        setPasskeys(result.data as unknown as Passkey[]);
      }
    } catch (error) {
      captureException(error);
    } finally {
      setIsLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchPasskeys();
  }, [fetchPasskeys]);

  const handleAdd = async () => {
    setIsAdding(true);
    try {
      const result = await authClient.passkey.addPasskey();

      if (result?.error) {
        toast.error(result.error.message ?? "Failed to add passkey.");
        setIsAdding(false);
        return;
      }

      toast.success("Passkey added successfully.");
      await fetchPasskeys();
    } catch (error) {
      captureException(error);
      toast.error("Failed to add passkey.");
    } finally {
      setIsAdding(false);
    }
  };

  const handleDelete = async (id: string) => {
    setDeletingId(id);
    try {
      const result = await authClient.passkey.deletePasskey({ id });

      if (result.error) {
        toast.error(result.error.message ?? "Failed to remove passkey.");
        setDeletingId(null);
        return;
      }

      toast.success("Passkey removed.");
      await fetchPasskeys();
    } catch (error) {
      captureException(error);
      toast.error("Failed to remove passkey.");
    } finally {
      setDeletingId(null);
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
          <Button disabled={isAdding} onClick={handleAdd} size="sm">
            {isAdding ? (
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
            {passkeys.map((passkey) => (
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
                  disabled={deletingId === passkey.id}
                  onClick={() => handleDelete(passkey.id)}
                  size="sm"
                  variant="destructive"
                >
                  {deletingId === passkey.id ? (
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
            ))}
          </div>
        )}
      </CardContent>
    </Card>
  );
};

export default PasskeyManagement;
