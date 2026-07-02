import { useIsHydrated } from "./use-is-hydrated";

export type HydratedTableData<T> = {
  data: T[];
  isHydrated: boolean;
  isLoading: boolean;
};

export function useHydratedTableData<T>(data: T[]): HydratedTableData<T> {
  const isHydrated = useIsHydrated();

  return {
    data: isHydrated ? data : [],
    isHydrated,
    isLoading: !isHydrated,
  };
}
