import { useEffect } from "react";

type NavigateForCreateQuery = (options: {
  replace: true;
  search: (previous: Record<string, unknown>) => Record<string, unknown>;
}) => void;

type PermissionGatedCreateQueryOptions = {
  canCreate: boolean;
  create: string | undefined;
  isReady: boolean;
  navigate: NavigateForCreateQuery;
  openCreateDialog: () => void;
};

export function usePermissionGatedCreateQuery({
  canCreate,
  create,
  isReady,
  navigate,
  openCreateDialog,
}: PermissionGatedCreateQueryOptions) {
  useEffect(() => {
    if (create !== "1" || !isReady) {
      return;
    }

    if (canCreate) {
      openCreateDialog();
    }
    navigate({
      search: (prev) => ({ ...prev, create: undefined }),
      replace: true,
    });
  }, [canCreate, create, isReady, navigate, openCreateDialog]);
}
