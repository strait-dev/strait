import { useEffect } from "react";

type PermissionGatedCreateQueryOptions = {
  canCreate: boolean;
  clearCreateQuery: () => void;
  create: string | undefined;
  isReady: boolean;
  openCreateDialog: () => void;
};

export function usePermissionGatedCreateQuery({
  canCreate,
  clearCreateQuery,
  create,
  isReady,
  openCreateDialog,
}: PermissionGatedCreateQueryOptions) {
  useEffect(() => {
    if (create !== "1" || !isReady) {
      return;
    }

    if (canCreate) {
      openCreateDialog();
    }
    clearCreateQuery();
  }, [canCreate, clearCreateQuery, create, isReady, openCreateDialog]);
}
