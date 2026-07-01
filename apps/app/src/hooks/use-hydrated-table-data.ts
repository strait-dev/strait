import { useEffect, useState } from "react";

export type HydratedTableData<T> = {
  data: T[];
  isHydrated: boolean;
  isLoading: boolean;
};

export function useHydratedTableData<T>(data: T[]): HydratedTableData<T> {
  const [isHydrated, setIsHydrated] = useState(false);

  useEffect(() => {
    setIsHydrated(true);
  }, []);

  return {
    data: isHydrated ? data : [],
    isHydrated,
    isLoading: !isHydrated,
  };
}
